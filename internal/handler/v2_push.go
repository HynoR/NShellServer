package handler

import (
	"net/http"

	"github.com/hynor/nshellserver/internal/db"
	"github.com/hynor/nshellserver/internal/model"
)

func (h *Handler) V2Push(w http.ResponseWriter, r *http.Request) {
	ws := WorkspaceName(r.Context())

	var req model.V2PushRequest
	if err := decodeJSON(r, &req); err != nil {
		h.logWarning(r, ws, "invalid v2 push request body")
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Operations) == 0 {
		h.logWarning(r, ws, "v2 push with empty operations")
		writeError(w, http.StatusBadRequest, "operations array is required")
		return
	}

	// Validate operation types.
	for _, op := range req.Operations {
		switch op.Type {
		case "upsertServer", "upsertSshKey":
			if len(op.Payload) == 0 {
				h.logWarning(r, ws, "v2 push upsert missing payload", "type", op.Type, "uuid", op.UUID)
				writeError(w, http.StatusBadRequest, "payload is required for upsert operations")
				return
			}
		case "deleteServer", "deleteSshKey":
			// ok
		default:
			h.logWarning(r, ws, "v2 push unknown operation type", "type", op.Type)
			writeError(w, http.StatusBadRequest, "unknown operation type: "+op.Type)
			return
		}
		if op.UUID == "" {
			h.logWarning(r, ws, "v2 push missing uuid", "type", op.Type)
			writeError(w, http.StatusBadRequest, "uuid is required")
			return
		}
	}

	// Convert to store operations.
	ops := make([]db.V2Operation, len(req.Operations))
	for i, op := range req.Operations {
		ops[i] = db.V2Operation{
			Type:         op.Type,
			UUID:         op.UUID,
			BaseRevision: op.BaseRevision,
			Payload:      op.Payload,
		}
	}

	result, err := h.Store.PushBatchV2(ws, req.BaseWorkspaceVersion, ops)
	if err != nil {
		if ce, ok := err.(*db.ConflictError); ok {
			h.logWarning(r, ws, "v2 push reference conflict", "error", ce.Message)
			writeError(w, http.StatusConflict, ce.Message)
			return
		}
		h.logError(r, ws, "failed to push v2 batch", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to push batch")
		return
	}

	if len(result.Conflicts) > 0 {
		h.logWarning(r, ws, "v2 push conflicts", "conflict_count", len(result.Conflicts))

		conflicts := make([]model.V2PushConflictItem, len(result.Conflicts))
		for i, c := range result.Conflicts {
			item := model.V2PushConflictItem{
				Type:           c.Type,
				ResourceType:   c.ResourceType,
				UUID:           c.UUID,
				ServerRevision: c.ServerRevision,
				ServerDeleted:  c.ServerDeleted,
				ServerPayload:  c.ServerPayload,
			}
			if c.ServerUpdatedAt != "" {
				s := c.ServerUpdatedAt
				item.ServerUpdatedAt = &s
			}
			conflicts[i] = item
		}

		writeJSON(w, http.StatusConflict, model.V2PushConflictResponse{
			OK:               false,
			Error:            "conflict",
			WorkspaceVersion: result.WorkspaceVersion,
			Conflicts:        conflicts,
		})
		return
	}

	h.logInfo(r, ws, "v2 push completed", "workspace_version", result.WorkspaceVersion, "operation_count", len(result.Results))

	results := make([]model.V2PushResultItem, len(result.Results))
	for i, r := range result.Results {
		results[i] = model.V2PushResultItem{
			Type:     r.Type,
			UUID:     r.UUID,
			Revision: r.Revision,
		}
	}

	writeJSON(w, http.StatusOK, model.V2PushSuccessResponse{
		OK:               true,
		WorkspaceVersion: result.WorkspaceVersion,
		Results:          results,
	})
}
