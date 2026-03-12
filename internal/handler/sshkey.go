package handler

import (
	"errors"
	"net/http"

	"github.com/hynor/nshellserver/internal/db"
	"github.com/hynor/nshellserver/internal/model"
)

func (h *Handler) UpsertSSHKey(w http.ResponseWriter, r *http.Request) {
	ws := WorkspaceName(r.Context())

	var req model.UpsertSSHKeyRequest
	if err := decodeJSON(r, &req); err != nil {
		h.logWarning(r, ws, "invalid ssh key upsert request")
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	id, err := extractID(req.SSHKey)
	if err != nil {
		h.logWarning(r, ws, "ssh key upsert missing resource id")
		writeError(w, http.StatusBadRequest, "sshKey must have an id field")
		return
	}

	workspaceVersion, resourceRevision, updatedAt, conflict, err := h.Store.UpsertSSHKey(ws, id, req.SSHKey, req.BaseRevision, req.Force)
	if err != nil {
		h.logError(r, ws, "failed to upsert ssh key", "resource_type", "ssh_key", "resource_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to upsert ssh key")
		return
	}
	if conflict != nil {
		h.logWarning(r, ws, "resource conflict", "resource_type", conflict.ResourceType, "resource_id", conflict.ResourceID, "server_revision", conflict.ServerRevision, "force", req.Force)
		writeJSON(w, http.StatusConflict, model.ConflictResponse{
			OK:    false,
			Error: "conflict",
			Conflict: model.ResourceConflictInfo{
				ResourceType:    conflict.ResourceType,
				ResourceID:      conflict.ResourceID,
				ServerRevision:  conflict.ServerRevision,
				ServerUpdatedAt: conflict.ServerUpdatedAt,
				ServerDeleted:   conflict.ServerDeleted,
				ServerPayload:   conflict.ServerPayload,
			},
		})
		return
	}

	h.logInfo(r, ws, "resource upserted", "resource_type", "ssh_key", "resource_id", id, "workspace_version", workspaceVersion, "resource_revision", resourceRevision, "force", req.Force)

	writeJSON(w, http.StatusOK, model.UpsertResponse{
		OK:               true,
		WorkspaceVersion: workspaceVersion,
		ResourceRevision: resourceRevision,
		UpdatedAt:        updatedAt,
	})
}

func (h *Handler) DeleteSSHKey(w http.ResponseWriter, r *http.Request) {
	ws := WorkspaceName(r.Context())

	var req model.DeleteRequest
	if err := decodeJSON(r, &req); err != nil {
		h.logWarning(r, ws, "invalid ssh key delete request")
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ID == "" {
		h.logWarning(r, ws, "ssh key delete missing resource id")
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	workspaceVersion, resourceRevision, deletedAt, conflict, err := h.Store.DeleteSSHKey(ws, req.ID, req.BaseRevision, req.Force)
	if err != nil {
		var ce *db.ConflictError
		if errors.As(err, &ce) {
			h.logWarning(r, ws, "resource delete blocked", "resource_type", "ssh_key", "resource_id", req.ID, "reason", ce.Message)
			writeError(w, http.StatusConflict, ce.Message)
			return
		}
		h.logError(r, ws, "failed to delete ssh key", "resource_type", "ssh_key", "resource_id", req.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete ssh key")
		return
	}
	if conflict != nil {
		h.logWarning(r, ws, "resource conflict", "resource_type", conflict.ResourceType, "resource_id", conflict.ResourceID, "server_revision", conflict.ServerRevision, "force", req.Force)
		writeJSON(w, http.StatusConflict, model.ConflictResponse{
			OK:    false,
			Error: "conflict",
			Conflict: model.ResourceConflictInfo{
				ResourceType:    conflict.ResourceType,
				ResourceID:      conflict.ResourceID,
				ServerRevision:  conflict.ServerRevision,
				ServerUpdatedAt: conflict.ServerUpdatedAt,
				ServerDeleted:   conflict.ServerDeleted,
				ServerPayload:   conflict.ServerPayload,
			},
		})
		return
	}

	h.logInfo(r, ws, "resource deleted", "resource_type", "ssh_key", "resource_id", req.ID, "workspace_version", workspaceVersion, "resource_revision", resourceRevision, "force", req.Force)

	writeJSON(w, http.StatusOK, model.DeleteResponse{
		OK:               true,
		WorkspaceVersion: workspaceVersion,
		ResourceRevision: resourceRevision,
		DeletedAt:        deletedAt,
	})
}
