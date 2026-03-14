package handler

import (
	"net/http"
	"time"

	"github.com/hynor/nshellserver/internal/db"
	"github.com/hynor/nshellserver/internal/model"
)

func (h *Handler) V2Pull(w http.ResponseWriter, r *http.Request) {
	ws := WorkspaceName(r.Context())

	var req model.V2PullRequest
	if err := decodeJSON(r, &req); err != nil {
		h.logWarning(r, ws, "invalid v2 pull request body")
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	snap, version, err := h.Store.PullSnapshotV2(ws)
	if err != nil {
		h.logError(r, ws, "failed to pull v2 snapshot", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to pull snapshot")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	h.logInfo(r, ws, "v2 pull completed", "known_version", req.KnownVersion, "server_version", version)

	writeJSON(w, http.StatusOK, model.V2PullResponse{
		OK:               true,
		WorkspaceVersion: version,
		ServerTime:       now,
		Servers:          mapV2ResourceSnapshots(snap.Servers),
		SSHKeys:          mapV2ResourceSnapshots(snap.SSHKeys),
		Deleted:          mapV2DeletedItems(snap.Deleted),
	})
}

func mapV2ResourceSnapshots(items []db.V2ResourceSnapshot) []model.V2ResourceSnapshot {
	result := make([]model.V2ResourceSnapshot, 0, len(items))
	for _, item := range items {
		result = append(result, model.V2ResourceSnapshot{
			UUID:      item.UUID,
			Revision:  item.Revision,
			UpdatedAt: item.UpdatedAt,
			Payload:   item.Payload,
		})
	}
	return result
}

func mapV2DeletedItems(items []db.V2DeletedResource) []model.V2DeletedItem {
	result := make([]model.V2DeletedItem, 0, len(items))
	for _, item := range items {
		result = append(result, model.V2DeletedItem{
			ResourceType: item.ResourceType,
			UUID:         item.UUID,
			Revision:     item.Revision,
			DeletedAt:    item.DeletedAt,
		})
	}
	return result
}
