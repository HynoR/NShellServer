package db

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func newTestStoreV2(t *testing.T) *Store {
	t.Helper()
	database, err := Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	store := NewStore(database)
	if err := store.EnsureWorkspaceV2("alice", strongWorkspacePassword); err != nil {
		t.Fatalf("ensure v2 workspace: %v", err)
	}
	return store
}

func TestV2WorkspaceIsolation(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	store := NewStore(database)

	// Create same workspace name in both v1 and v2.
	if err := store.EnsureWorkspace("shared", strongWorkspacePassword); err != nil {
		t.Fatalf("ensure v1 workspace: %v", err)
	}
	if err := store.EnsureWorkspaceV2("shared", "different-password-123"); err != nil {
		t.Fatalf("ensure v2 workspace: %v", err)
	}

	// v1 and v2 should have independent versions.
	v1Version, err := store.GetVersion("shared")
	if err != nil {
		t.Fatalf("get v1 version: %v", err)
	}
	v2Version, err := store.GetVersionV2("shared")
	if err != nil {
		t.Fatalf("get v2 version: %v", err)
	}
	if v1Version != 0 || v2Version != 0 {
		t.Fatalf("expected both versions 0, got v1=%d v2=%d", v1Version, v2Version)
	}

	// Push to v2 should not affect v1 version.
	_, err = store.PushBatchV2("shared", 0, []V2Operation{
		{Type: "upsertServer", UUID: "s1", Payload: json.RawMessage(`{"name":"srv"}`)},
	})
	if err != nil {
		t.Fatalf("push v2: %v", err)
	}

	v1Version, _ = store.GetVersion("shared")
	v2Version, _ = store.GetVersionV2("shared")
	if v1Version != 0 {
		t.Fatalf("v1 version should be unchanged, got %d", v1Version)
	}
	if v2Version != 1 {
		t.Fatalf("v2 version should be 1, got %d", v2Version)
	}
}

func TestV2PushBatchSuccess(t *testing.T) {
	store := newTestStoreV2(t)

	result, err := store.PushBatchV2("alice", 0, []V2Operation{
		{Type: "upsertServer", UUID: "srv-1", Payload: json.RawMessage(`{"name":"prod-a","host":"1.2.3.4","port":22}`)},
		{Type: "upsertSshKey", UUID: "key-1", Payload: json.RawMessage(`{"name":"my-key","keyCipher":"abc"}`)},
	})
	if err != nil {
		t.Fatalf("push batch: %v", err)
	}
	if len(result.Conflicts) > 0 {
		t.Fatalf("expected no conflicts, got %d", len(result.Conflicts))
	}
	if result.WorkspaceVersion != 1 {
		t.Fatalf("expected workspace version 1, got %d", result.WorkspaceVersion)
	}
	if len(result.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result.Results))
	}
	if result.Results[0].Revision != 1 || result.Results[1].Revision != 1 {
		t.Fatalf("expected both revisions 1, got %d and %d", result.Results[0].Revision, result.Results[1].Revision)
	}
}

func TestV2PushBatchConflictRollsBack(t *testing.T) {
	store := newTestStoreV2(t)

	// Initial push.
	_, err := store.PushBatchV2("alice", 0, []V2Operation{
		{Type: "upsertServer", UUID: "srv-1", Payload: json.RawMessage(`{"name":"prod-a"}`)},
	})
	if err != nil {
		t.Fatalf("initial push: %v", err)
	}

	// Push with stale baseRevision should conflict and roll back.
	staleRev := int64Ptr(0) // server is at revision 1
	result, err := store.PushBatchV2("alice", 1, []V2Operation{
		{Type: "upsertServer", UUID: "srv-1", BaseRevision: staleRev, Payload: json.RawMessage(`{"name":"prod-b"}`)},
		{Type: "upsertServer", UUID: "srv-2", Payload: json.RawMessage(`{"name":"prod-c"}`)},
	})
	if err != nil {
		t.Fatalf("conflict push: %v", err)
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(result.Conflicts))
	}
	if result.Conflicts[0].UUID != "srv-1" {
		t.Fatalf("expected conflict on srv-1, got %s", result.Conflicts[0].UUID)
	}
	if result.Conflicts[0].ServerRevision != 1 {
		t.Fatalf("expected server revision 1, got %d", result.Conflicts[0].ServerRevision)
	}

	// srv-2 should NOT have been created (rollback).
	snap, version, err := store.PullSnapshotV2("alice")
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected version 1 (unchanged), got %d", version)
	}
	if len(snap.Servers) != 1 {
		t.Fatalf("expected 1 server (srv-2 rolled back), got %d", len(snap.Servers))
	}
}

func TestV2PullSnapshot(t *testing.T) {
	store := newTestStoreV2(t)

	// Push some data.
	_, err := store.PushBatchV2("alice", 0, []V2Operation{
		{Type: "upsertServer", UUID: "srv-1", Payload: json.RawMessage(`{"name":"prod-a","host":"1.2.3.4"}`)},
		{Type: "upsertSshKey", UUID: "key-1", Payload: json.RawMessage(`{"name":"my-key"}`)},
	})
	if err != nil {
		t.Fatalf("push: %v", err)
	}

	// Delete the server.
	_, err = store.PushBatchV2("alice", 1, []V2Operation{
		{Type: "deleteServer", UUID: "srv-1", BaseRevision: int64Ptr(1)},
	})
	if err != nil {
		t.Fatalf("delete push: %v", err)
	}

	snap, version, err := store.PullSnapshotV2("alice")
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if version != 2 {
		t.Fatalf("expected version 2, got %d", version)
	}
	if len(snap.Servers) != 0 {
		t.Fatalf("expected 0 servers, got %d", len(snap.Servers))
	}
	if len(snap.SSHKeys) != 1 {
		t.Fatalf("expected 1 ssh key, got %d", len(snap.SSHKeys))
	}
	if snap.SSHKeys[0].UUID != "key-1" {
		t.Fatalf("expected key-1, got %s", snap.SSHKeys[0].UUID)
	}
	if len(snap.Deleted) != 1 {
		t.Fatalf("expected 1 deleted, got %d", len(snap.Deleted))
	}
	if snap.Deleted[0].ResourceType != "server" || snap.Deleted[0].UUID != "srv-1" {
		t.Fatalf("expected deleted server srv-1, got %s %s", snap.Deleted[0].ResourceType, snap.Deleted[0].UUID)
	}
}

func TestV2DeleteSshKeyReferencedByServer(t *testing.T) {
	store := newTestStoreV2(t)

	// Create a server that references a key.
	_, err := store.PushBatchV2("alice", 0, []V2Operation{
		{Type: "upsertSshKey", UUID: "key-1", Payload: json.RawMessage(`{"name":"my-key"}`)},
		{Type: "upsertServer", UUID: "srv-1", Payload: json.RawMessage(`{"name":"prod","sshKeyUuid":"key-1"}`)},
	})
	if err != nil {
		t.Fatalf("push: %v", err)
	}

	// Try to delete the referenced key.
	_, err = store.PushBatchV2("alice", 1, []V2Operation{
		{Type: "deleteSshKey", UUID: "key-1", BaseRevision: int64Ptr(1)},
	})
	if err == nil {
		t.Fatalf("expected error deleting referenced ssh key")
	}
	if _, ok := err.(*ConflictError); !ok {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}
}

func TestV2VersionIncrementsOncePerPush(t *testing.T) {
	store := newTestStoreV2(t)

	result, err := store.PushBatchV2("alice", 0, []V2Operation{
		{Type: "upsertServer", UUID: "srv-1", Payload: json.RawMessage(`{"name":"a"}`)},
		{Type: "upsertServer", UUID: "srv-2", Payload: json.RawMessage(`{"name":"b"}`)},
		{Type: "upsertServer", UUID: "srv-3", Payload: json.RawMessage(`{"name":"c"}`)},
	})
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if result.WorkspaceVersion != 1 {
		t.Fatalf("expected workspace version 1 (single increment), got %d", result.WorkspaceVersion)
	}
}

func TestV2UpsertResurrectsDeletedResource(t *testing.T) {
	store := newTestStoreV2(t)

	// Create then delete.
	_, err := store.PushBatchV2("alice", 0, []V2Operation{
		{Type: "upsertServer", UUID: "srv-1", Payload: json.RawMessage(`{"name":"original"}`)},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = store.PushBatchV2("alice", 1, []V2Operation{
		{Type: "deleteServer", UUID: "srv-1", BaseRevision: int64Ptr(1)},
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Resurrect with null baseRevision (new create from client's perspective after conflict resolution).
	result, err := store.PushBatchV2("alice", 2, []V2Operation{
		{Type: "upsertServer", UUID: "srv-1", BaseRevision: int64Ptr(2), Payload: json.RawMessage(`{"name":"resurrected"}`)},
	})
	if err != nil {
		t.Fatalf("resurrect: %v", err)
	}
	if len(result.Conflicts) > 0 {
		t.Fatalf("expected no conflicts, got %d", len(result.Conflicts))
	}

	snap, _, err := store.PullSnapshotV2("alice")
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if len(snap.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(snap.Servers))
	}
	if len(snap.Deleted) != 0 {
		t.Fatalf("expected 0 deleted, got %d", len(snap.Deleted))
	}
}

func TestV2RejectsShortWorkspacePassword(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	store := NewStore(database)
	if err := store.EnsureWorkspaceV2("alice", "short"); err == nil {
		t.Fatalf("expected short password to be rejected")
	}
}

func TestV2ConflictOnDeletedResourceUpsert(t *testing.T) {
	store := newTestStoreV2(t)

	// Create and delete a server.
	_, err := store.PushBatchV2("alice", 0, []V2Operation{
		{Type: "upsertServer", UUID: "srv-1", Payload: json.RawMessage(`{"name":"original"}`)},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = store.PushBatchV2("alice", 1, []V2Operation{
		{Type: "deleteServer", UUID: "srv-1", BaseRevision: int64Ptr(1)},
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Client tries to upsert with baseRevision=1, but server has revision 2 (deleted).
	result, err := store.PushBatchV2("alice", 2, []V2Operation{
		{Type: "upsertServer", UUID: "srv-1", BaseRevision: int64Ptr(1), Payload: json.RawMessage(`{"name":"updated"}`)},
	})
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(result.Conflicts))
	}
	if !result.Conflicts[0].ServerDeleted {
		t.Fatalf("expected serverDeleted=true")
	}
	if result.Conflicts[0].ServerRevision != 2 {
		t.Fatalf("expected server revision 2, got %d", result.Conflicts[0].ServerRevision)
	}
}
