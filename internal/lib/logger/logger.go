package logger

import (
	"log/slog"
	"os"
	"strings"

	"github.com/ssovich/diasoft-gateway/internal/lib/logger/slogpretty"
)

func New(level string) *slog.Logger {
	var slogLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	handler := slogpretty.PrettyHandlerOptions{
		SlogOpts: &slog.HandlerOptions{Level: slogLevel},
	}.NewPrettyHandler(os.Stdout)

	return slog.New(handler)
}
