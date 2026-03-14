package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ---------- v2 Workspace ----------

func (s *Store) EnsureWorkspaceV2(name, password string) error {
	var storedHash string
	err := s.db.QueryRow(
		`SELECT password_hash FROM v2_workspaces WHERE workspace_name = ?`, name,
	).Scan(&storedHash)

	if err == sql.ErrNoRows {
		if len(password) < 8 {
			return ErrWorkspacePasswordTooShort
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("bcrypt hash: %w", err)
		}
		_, err = s.db.Exec(
			`INSERT INTO v2_workspaces (workspace_name, password_hash) VALUES (?, ?)`,
			name, string(hash),
		)
		if err != nil {
			return fmt.Errorf("insert v2 workspace: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("fetch v2 workspace: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password)); err != nil {
		return ErrInvalidPassword
	}
	return nil
}

func (s *Store) GetVersionV2(workspace string) (int64, error) {
	var v int64
	err := s.db.QueryRow(
		`SELECT version FROM v2_workspaces WHERE workspace_name = ?`, workspace,
	).Scan(&v)
	return v, err
}

// ---------- v2 Pull ----------

type V2SnapshotData struct {
	Servers []V2ResourceSnapshot
	SSHKeys []V2ResourceSnapshot
	Deleted []V2DeletedResource
}

type V2ResourceSnapshot struct {
	UUID      string
	Payload   json.RawMessage
	Revision  int64
	UpdatedAt string
}

type V2DeletedResource struct {
	ResourceType string
	UUID         string
	Revision     int64
	DeletedAt    string
}

func (s *Store) PullSnapshotV2(workspace string) (*V2SnapshotData, int64, error) {
	version, err := s.GetVersionV2(workspace)
	if err != nil {
		return nil, 0, err
	}

	snap := &V2SnapshotData{}

	snap.Servers, err = s.listV2Payloads("v2_servers", workspace)
	if err != nil {
		return nil, 0, err
	}
	snap.SSHKeys, err = s.listV2Payloads("v2_ssh_keys", workspace)
	if err != nil {
		return nil, 0, err
	}
	snap.Deleted, err = s.listV2Tombstones(workspace)
	if err != nil {
		return nil, 0, err
	}

	return snap, version, nil
}

func (s *Store) listV2Payloads(table, workspace string) ([]V2ResourceSnapshot, error) {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT uuid, payload_json, revision, updated_at FROM %s WHERE workspace_name = ? ORDER BY uuid`, table), workspace,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []V2ResourceSnapshot
	for rows.Next() {
		var item V2ResourceSnapshot
		var payload string
		if err := rows.Scan(&item.UUID, &payload, &item.Revision, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Payload = json.RawMessage(payload)
		result = append(result, item)
	}
	if result == nil {
		result = []V2ResourceSnapshot{}
	}
	return result, rows.Err()
}

func (s *Store) listV2Tombstones(workspace string) ([]V2DeletedResource, error) {
	rows, err := s.db.Query(
		`SELECT resource_type, resource_uuid, revision, deleted_at FROM v2_deleted_tombstones WHERE workspace_name = ? ORDER BY resource_type, resource_uuid`,
		workspace,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []V2DeletedResource
	for rows.Next() {
		var item V2DeletedResource
		if err := rows.Scan(&item.ResourceType, &item.UUID, &item.Revision, &item.DeletedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if result == nil {
		result = []V2DeletedResource{}
	}
	return result, rows.Err()
}

// ---------- v2 Push ----------

type V2PushResult struct {
	Type     string
	UUID     string
	Revision int64
}

type V2PushConflict struct {
	Type            string
	ResourceType    string
	UUID            string
	ServerRevision  int64
	ServerDeleted   bool
	ServerUpdatedAt string
	ServerPayload   json.RawMessage
}

type V2PushBatchResult struct {
	WorkspaceVersion int64
	Results          []V2PushResult
	Conflicts        []V2PushConflict
}

type V2Operation struct {
	Type         string
	UUID         string
	BaseRevision *int64
	Payload      json.RawMessage
}

func (s *Store) PushBatchV2(workspace string, baseWorkspaceVersion int64, operations []V2Operation) (*V2PushBatchResult, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)

	// First pass: detect conflicts for all operations.
	var conflicts []V2PushConflict
	for _, op := range operations {
		table, tombstoneType, resourceType, err := v2ResolveOpType(op.Type)
		if err != nil {
			return nil, err
		}

		current, err := s.getV2ResourceStateTx(tx, table, tombstoneType, workspace, op.UUID)
		if err != nil {
			return nil, err
		}

		if conflict := validateResourceRevision(resourceType, op.UUID, current, op.BaseRevision, false); conflict != nil {
			c := V2PushConflict{
				Type:           op.Type,
				ResourceType:   resourceType,
				UUID:           op.UUID,
				ServerRevision: conflict.ServerRevision,
				ServerDeleted:  conflict.ServerDeleted,
				ServerUpdatedAt: conflict.ServerUpdatedAt,
				ServerPayload:  conflict.ServerPayload,
			}
			conflicts = append(conflicts, c)
		}
	}

	if len(conflicts) > 0 {
		// Read current workspace version for the conflict response.
		version, err := s.getV2VersionTx(tx, workspace)
		if err != nil {
			return nil, err
		}
		return &V2PushBatchResult{
			WorkspaceVersion: version,
			Conflicts:        conflicts,
		}, nil
	}

	// Second pass: execute all operations (no conflicts).
	var results []V2PushResult
	for _, op := range operations {
		table, tombstoneType, _, err := v2ResolveOpType(op.Type)
		if err != nil {
			return nil, err
		}

		current, err := s.getV2ResourceStateTx(tx, table, tombstoneType, workspace, op.UUID)
		if err != nil {
			return nil, err
		}

		nextRevision := current.NextRevision()

		switch {
		case isV2Upsert(op.Type):
			if err := s.execV2Upsert(tx, table, tombstoneType, workspace, op.UUID, op.Payload, nextRevision, now, current); err != nil {
				return nil, err
			}
		case isV2Delete(op.Type):
			if op.Type == "deleteSshKey" {
				if err := s.checkV2SshKeyReferences(tx, workspace, op.UUID); err != nil {
					return nil, err
				}
			}
			if err := s.execV2Delete(tx, table, tombstoneType, workspace, op.UUID, nextRevision, now, current); err != nil {
				return nil, err
			}
		}

		results = append(results, V2PushResult{
			Type:     op.Type,
			UUID:     op.UUID,
			Revision: nextRevision,
		})
	}

	// Increment workspace version once for the entire batch.
	version, err := s.incrementV2Version(tx, workspace, now)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &V2PushBatchResult{
		WorkspaceVersion: version,
		Results:          results,
	}, nil
}

// ---------- v2 Helpers ----------

func v2ResolveOpType(opType string) (table, tombstoneType, resourceType string, err error) {
	switch opType {
	case "upsertServer", "deleteServer":
		return "v2_servers", "server", "server", nil
	case "upsertSshKey", "deleteSshKey":
		return "v2_ssh_keys", "sshKey", "sshKey", nil
	default:
		return "", "", "", fmt.Errorf("unknown v2 operation type: %s", opType)
	}
}

func isV2Upsert(opType string) bool {
	return opType == "upsertServer" || opType == "upsertSshKey"
}

func isV2Delete(opType string) bool {
	return opType == "deleteServer" || opType == "deleteSshKey"
}

func (s *Store) getV2ResourceStateTx(tx *sql.Tx, table, tombstoneType, workspace, uuid string) (resourceState, error) {
	var state resourceState
	var payload string
	err := tx.QueryRow(
		fmt.Sprintf(`SELECT payload_json, revision, updated_at FROM %s WHERE workspace_name = ? AND uuid = ?`, table),
		workspace, uuid,
	).Scan(&payload, &state.Revision, &state.UpdatedAt)
	switch {
	case err == nil:
		state.Exists = true
		state.Payload = json.RawMessage(payload)
		return state, nil
	case err != sql.ErrNoRows:
		return resourceState{}, fmt.Errorf("get v2 %s state: %w", table, err)
	}

	err = tx.QueryRow(
		`SELECT revision, deleted_at FROM v2_deleted_tombstones WHERE workspace_name = ? AND resource_type = ? AND resource_uuid = ?`,
		workspace, tombstoneType, uuid,
	).Scan(&state.Revision, &state.UpdatedAt)
	switch {
	case err == nil:
		state.Exists = true
		state.Deleted = true
		return state, nil
	case err == sql.ErrNoRows:
		return resourceState{}, nil
	default:
		return resourceState{}, fmt.Errorf("get v2 tombstone state: %w", err)
	}
}

func (s *Store) getV2VersionTx(tx *sql.Tx, workspace string) (int64, error) {
	var v int64
	err := tx.QueryRow(
		`SELECT version FROM v2_workspaces WHERE workspace_name = ?`, workspace,
	).Scan(&v)
	return v, err
}

func (s *Store) incrementV2Version(tx *sql.Tx, workspace, now string) (int64, error) {
	_, err := tx.Exec(
		`UPDATE v2_workspaces SET version = version + 1, updated_at = ? WHERE workspace_name = ?`,
		now, workspace,
	)
	if err != nil {
		return 0, fmt.Errorf("increment v2 version: %w", err)
	}

	var version int64
	err = tx.QueryRow(
		`SELECT version FROM v2_workspaces WHERE workspace_name = ?`, workspace,
	).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("get v2 version: %w", err)
	}
	return version, nil
}

func (s *Store) execV2Upsert(tx *sql.Tx, table, tombstoneType, workspace, uuid string, payload json.RawMessage, nextRevision int64, now string, current resourceState) error {
	if current.Exists && !current.Deleted {
		_, err := tx.Exec(
			fmt.Sprintf(`UPDATE %s SET payload_json = ?, revision = ?, updated_at = ? WHERE workspace_name = ? AND uuid = ?`, table),
			string(payload), nextRevision, now, workspace, uuid,
		)
		if err != nil {
			return fmt.Errorf("update v2 %s: %w", table, err)
		}
	} else {
		_, err := tx.Exec(
			fmt.Sprintf(`INSERT INTO %s (workspace_name, uuid, payload_json, revision, updated_at) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (workspace_name, uuid) DO UPDATE SET payload_json = excluded.payload_json, revision = excluded.revision, updated_at = excluded.updated_at`, table),
			workspace, uuid, string(payload), nextRevision, now,
		)
		if err != nil {
			return fmt.Errorf("insert v2 %s: %w", table, err)
		}
	}

	// Remove tombstone if resurrecting.
	_, err := tx.Exec(
		`DELETE FROM v2_deleted_tombstones WHERE workspace_name = ? AND resource_type = ? AND resource_uuid = ?`,
		workspace, tombstoneType, uuid,
	)
	if err != nil {
		return fmt.Errorf("delete v2 tombstone: %w", err)
	}
	return nil
}

func (s *Store) execV2Delete(tx *sql.Tx, table, tombstoneType, workspace, uuid string, nextRevision int64, now string, current resourceState) error {
	if current.Exists && !current.Deleted {
		_, err := tx.Exec(
			fmt.Sprintf(`DELETE FROM %s WHERE workspace_name = ? AND uuid = ?`, table),
			workspace, uuid,
		)
		if err != nil {
			return fmt.Errorf("delete v2 %s: %w", table, err)
		}
	}

	_, err := tx.Exec(
		`INSERT OR REPLACE INTO v2_deleted_tombstones (workspace_name, resource_type, resource_uuid, revision, deleted_at) VALUES (?, ?, ?, ?, ?)`,
		workspace, tombstoneType, uuid, nextRevision, now,
	)
	if err != nil {
		return fmt.Errorf("insert v2 tombstone: %w", err)
	}
	return nil
}

func (s *Store) checkV2SshKeyReferences(tx *sql.Tx, workspace, uuid string) error {
	var count int
	err := tx.QueryRow(
		`SELECT COUNT(*) FROM v2_servers WHERE workspace_name = ? AND json_extract(payload_json, '$.sshKeyUuid') = ?`,
		workspace, uuid,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check v2 ssh key refs: %w", err)
	}
	if count > 0 {
		return &ConflictError{Message: fmt.Sprintf("ssh key %q is still referenced by %d server(s)", uuid, count)}
	}
	return nil
}
