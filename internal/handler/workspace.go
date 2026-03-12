package handler

import (
	"net/http"
	"time"

	"github.com/hynor/nshellserver/internal/model"
)

func (h *Handler) WorkspaceStatus(w http.ResponseWriter, r *http.Request) {
	ws := WorkspaceName(r.Context())

	version, err := h.Store.GetVersion(ws)
	if err != nil {
		h.logError(r, ws, "failed to get workspace version", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get workspace version")
		return
	}

	h.logInfo(r, ws, "workspace status checked", "version", version)

	writeJSON(w, http.StatusOK, model.WorkspaceStatusResponse{
		OK:         true,
		Workspace:  ws,
		Version:    version,
		ServerTime: time.Now().UTC().Format(time.RFC3339Nano),
	})
}
