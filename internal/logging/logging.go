package logging

import (
	"io"
	"log/slog"
	"strings"
)

func ResolveLevel(raw string) (slog.Level, string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "info":
		return slog.LevelInfo, "info", false
	case "error":
		return slog.LevelError, "error", false
	case "warning", "warn", "warnning":
		return slog.LevelWarn, "warning", false
	case "debug":
		return slog.LevelDebug, "debug", false
	default:
		return slog.LevelInfo, "info", true
	}
}

func NewLogger(w io.Writer, level string) *slog.Logger {
	resolved, _, _ := ResolveLevel(level)
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: resolved,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key != slog.LevelKey {
				return attr
			}

			label := strings.ToLower(attr.Value.String())
			if label == "warn" {
				label = "warning"
			}
			attr.Value = slog.StringValue(label)
			return attr
		},
	}))
}
