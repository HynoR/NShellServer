package handler

import (
	"errors"
	"net/http"

	"github.com/hynor/nshellserver/internal/db"
	"github.com/hynor/nshellserver/internal/model"
)

func (h *Handler) UpsertProxy(w http.ResponseWriter, r *http.Request) {
	ws := WorkspaceName(r.Context())

	var req model.UpsertProxyRequest
	if err := decodeJSON(r, &req); err != nil {
		h.logWarning(r, ws, "invalid proxy upsert request")
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	id, err := extractID(req.Proxy)
	if err != nil {
		h.logWarning(r, ws, "proxy upsert missing resource id")
		writeError(w, http.StatusBadRequest, "proxy must have an id field")
		return
	}

	workspaceVersion, resourceRevision, updatedAt, conflict, err := h.Store.UpsertProxy(ws, id, req.Proxy, req.BaseRevision, req.Force)
	if err != nil {
		h.logError(r, ws, "failed to upsert proxy", "resource_type", "proxy", "resource_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to upsert proxy")
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

	h.logInfo(r, ws, "resource upserted", "resource_type", "proxy", "resource_id", id, "workspace_version", workspaceVersion, "resource_revision", resourceRevision, "force", req.Force)

	writeJSON(w, http.StatusOK, model.UpsertResponse{
		OK:               true,
		WorkspaceVersion: workspaceVersion,
		ResourceRevision: resourceRevision,
		UpdatedAt:        updatedAt,
	})
}

func (h *Handler) DeleteProxy(w http.ResponseWriter, r *http.Request) {
	ws := WorkspaceName(r.Context())

	var req model.DeleteRequest
	if err := decodeJSON(r, &req); err != nil {
		h.logWarning(r, ws, "invalid proxy delete request")
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ID == "" {
		h.logWarning(r, ws, "proxy delete missing resource id")
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	workspaceVersion, resourceRevision, deletedAt, conflict, err := h.Store.DeleteProxy(ws, req.ID, req.BaseRevision, req.Force)
	if err != nil {
		var ce *db.ConflictError
		if errors.As(err, &ce) {
			h.logWarning(r, ws, "resource delete blocked", "resource_type", "proxy", "resource_id", req.ID, "reason", ce.Message)
			writeError(w, http.StatusConflict, ce.Message)
			return
		}
		h.logError(r, ws, "failed to delete proxy", "resource_type", "proxy", "resource_id", req.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete proxy")
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

	h.logInfo(r, ws, "resource deleted", "resource_type", "proxy", "resource_id", req.ID, "workspace_version", workspaceVersion, "resource_revision", resourceRevision, "force", req.Force)

	writeJSON(w, http.StatusOK, model.DeleteResponse{
		OK:               true,
		WorkspaceVersion: workspaceVersion,
		ResourceRevision: resourceRevision,
		DeletedAt:        deletedAt,
	})
}
