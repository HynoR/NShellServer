package handler

import (
	"net/http"
	"time"

	"github.com/hynor/nshellserver/internal/model"
)

func (h *Handler) V2Resolve(w http.ResponseWriter, r *http.Request) {
	ws := WorkspaceName(r.Context())

	var req model.V2ResolveRequest
	if err := decodeJSON(r, &req); err != nil {
		h.logWarning(r, ws, "invalid v2 resolve request body")
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	version, err := h.Store.GetVersionV2(ws)
	if err != nil {
		h.logError(r, ws, "failed to get v2 workspace version", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get workspace version")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	h.logInfo(r, ws, "v2 resolve completed", "version", version)

	writeJSON(w, http.StatusOK, model.V2ResolveResponse{
		OK:             true,
		WorkspaceID:    ws,
		ServerTime:     now,
		CurrentVersion: version,
		Capabilities: model.V2Capabilities{
			ResourceTypes:     []string{"server", "sshKey"},
			SupportsBatchPush: true,
		},
	})
}
