package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/hynor/nshellserver/internal/db"
)

func (h *Handler) V2AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username == "" || password == "" {
			h.Logger.Warn("missing credentials", "client_ip", clientIP(r.RemoteAddr), "path", r.URL.Path)
			w.Header().Set("WWW-Authenticate", `Basic realm="nshell"`)
			writeError(w, http.StatusUnauthorized, "missing credentials")
			return
		}

		authScope := authFailureScope(username, clientIP(r.RemoteAddr))

		if h.RateLimiter.IsLockedOut(authScope) {
			h.logWarning(r, username, "workspace auth locked out")
			w.Header().Set("Retry-After", "900")
			writeError(w, http.StatusTooManyRequests, "too many auth failures, try again later")
			return
		}

		if err := h.Store.EnsureWorkspaceV2(username, password); err != nil {
			if errors.Is(err, db.ErrInvalidPassword) || errors.Is(err, db.ErrWorkspacePasswordTooShort) {
				locked := h.RateLimiter.RecordAuthFailure(authScope)
				h.logWarning(r, username, "authentication failed")
				if locked {
					h.logWarning(r, username, "workspace locked due to auth failures")
				}
				writeError(w, http.StatusUnauthorized, "invalid credentials")
			} else {
				h.logError(r, username, "auth error", "error", err)
				writeError(w, http.StatusInternalServerError, "auth error")
			}
			return
		}

		ctx := context.WithValue(r.Context(), workspaceKey, username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
