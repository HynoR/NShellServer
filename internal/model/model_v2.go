package model

import "encoding/json"

// ---------- v2 Resolve ----------

type V2ResolveRequest struct {
	WorkspaceName string `json:"workspaceName"`
}

type V2ResolveResponse struct {
	OK             bool             `json:"ok"`
	WorkspaceID    string           `json:"workspaceId"`
	ServerTime     string           `json:"serverTime"`
	CurrentVersion int64            `json:"currentVersion"`
	Capabilities   V2Capabilities   `json:"capabilities"`
}

type V2Capabilities struct {
	ResourceTypes    []string `json:"resourceTypes"`
	SupportsBatchPush bool    `json:"supportsBatchPush"`
}

// ---------- v2 Pull ----------

type V2PullRequest struct {
	KnownVersion int64 `json:"knownVersion"`
}

type V2PullResponse struct {
	OK               bool                  `json:"ok"`
	WorkspaceVersion int64                 `json:"workspaceVersion"`
	ServerTime       string                `json:"serverTime"`
	Servers          []V2ResourceSnapshot   `json:"servers"`
	SSHKeys          []V2ResourceSnapshot   `json:"sshKeys"`
	Deleted          []V2DeletedItem        `json:"deleted"`
}

type V2ResourceSnapshot struct {
	UUID      string          `json:"uuid"`
	Revision  int64           `json:"revision"`
	UpdatedAt string          `json:"updatedAt"`
	Payload   json.RawMessage `json:"payload"`
}

type V2DeletedItem struct {
	ResourceType string `json:"resourceType"`
	UUID         string `json:"uuid"`
	Revision     int64  `json:"revision"`
	DeletedAt    string `json:"deletedAt"`
}

// ---------- v2 Push ----------

type V2PushRequest struct {
	BaseWorkspaceVersion int64             `json:"baseWorkspaceVersion"`
	Operations           []V2PushOperation `json:"operations"`
}

type V2PushOperation struct {
	Type         string          `json:"type"`
	UUID         string          `json:"uuid"`
	BaseRevision *int64          `json:"baseRevision"`
	Payload      json.RawMessage `json:"payload,omitempty"`
}

type V2PushSuccessResponse struct {
	OK               bool               `json:"ok"`
	WorkspaceVersion int64              `json:"workspaceVersion"`
	Results          []V2PushResultItem `json:"results"`
}

type V2PushResultItem struct {
	Type     string `json:"type"`
	UUID     string `json:"uuid"`
	Revision int64  `json:"revision"`
}

type V2PushConflictResponse struct {
	OK               bool                 `json:"ok"`
	Error            string               `json:"error"`
	WorkspaceVersion int64                `json:"workspaceVersion"`
	Conflicts        []V2PushConflictItem `json:"conflicts"`
}

type V2PushConflictItem struct {
	Type            string          `json:"type"`
	ResourceType    string          `json:"resourceType"`
	UUID            string          `json:"uuid"`
	ServerRevision  int64           `json:"serverRevision"`
	ServerDeleted   bool            `json:"serverDeleted"`
	ServerUpdatedAt *string         `json:"serverUpdatedAt"`
	ServerPayload   json.RawMessage `json:"serverPayload,omitempty"`
}
