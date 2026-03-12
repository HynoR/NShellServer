package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/hynor/nshellserver/internal/db"
	"github.com/hynor/nshellserver/internal/logging"
)

type Handler struct {
	Store       *db.Store
	RateLimiter *RateLimiter
	Logger      *slog.Logger
}

func New(store *db.Store, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = logging.NewLogger(io.Discard, "info")
	}
	return &Handler{Store: store, RateLimiter: NewRateLimiter(logger), Logger: logger}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"ok":    false,
		"error": msg,
	})
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
