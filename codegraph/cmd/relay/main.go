package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/humanbeeng/lepo/prototypes/codegraph/assistant"
)

var (
	version = "dev"
)

func main() {
	cfg, err := assistant.LoadConfig()
	if err != nil {
		slog.Error("failed to load assistant configuration", "err", err)
		os.Exit(1)
	}

	fmt.Printf("Relay version: %s\n", version)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := assistant.Run(ctx, cfg); err != nil {
		slog.Error("assistant terminated with error", "err", err)
		os.Exit(1)
	}
}
