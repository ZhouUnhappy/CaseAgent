// Package logging configures the process-wide slog default logger.
//
// Handler / service code can just call slog.Info / slog.Error directly with
// key-value attrs (e.g. slog.Info("task created", "task_id", id)). Init reads
// CASEAGENT_LOG_LEVEL (debug/info/warn/error, default info) and
// CASEAGENT_LOG_JSON (true switches to JSONHandler, default TextHandler) so
// the operator can flip output format without changing call sites.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

func Init() {
	level := parseLevel(os.Getenv("CASEAGENT_LOG_LEVEL"))
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if strings.EqualFold(os.Getenv("CASEAGENT_LOG_JSON"), "true") {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}

func parseLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
