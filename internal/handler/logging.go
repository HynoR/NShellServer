package handler

import (
	"context"
	"net/http"
	"time"
)

func workspaceNameFromContext(ctx context.Context) (string, bool) {
	ws, ok := ctx.Value(workspaceKey).(string)
	return ws, ok
}

func (h *Handler) RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)

		fields := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.statusCode(),
			"duration_ms", time.Since(start).Milliseconds(),
			"client_ip", clientIP(r.RemoteAddr),
		}
		if ws, ok := workspaceNameFromContext(r.Context()); ok {
			fields = append(fields, "workspace", ws)
		}
		h.Logger.Debug("request completed", fields...)
	})
}

func (h *Handler) requestFields(r *http.Request, workspace string, attrs ...any) []any {
	fields := []any{
		"workspace", workspace,
		"client_ip", clientIP(r.RemoteAddr),
		"path", r.URL.Path,
	}
	return append(fields, attrs...)
}

func (h *Handler) logInfo(r *http.Request, workspace, msg string, attrs ...any) {
	h.Logger.Info(msg, h.requestFields(r, workspace, attrs...)...)
}

func (h *Handler) logWarning(r *http.Request, workspace, msg string, attrs ...any) {
	h.Logger.Warn(msg, h.requestFields(r, workspace, attrs...)...)
}

func (h *Handler) logError(r *http.Request, workspace, msg string, attrs ...any) {
	h.Logger.Error(msg, h.requestFields(r, workspace, attrs...)...)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) statusCode() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}
