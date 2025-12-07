package logger

import (
	"log/slog"
	"os"

	"basegraph.app/relay/core/config"
)

// Setup initializes the default logger based on the configuration.
// In production, it uses JSON formatting with Info level.
// In development, it uses text formatting with Debug level.
func Setup(cfg config.Config) {
	var handler slog.Handler
	if cfg.IsProduction() {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	}
	slog.SetDefault(slog.New(handler))
}

