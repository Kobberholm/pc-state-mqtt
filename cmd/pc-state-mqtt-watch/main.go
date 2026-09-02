package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"pc-state-mqtt/internal/watcher"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(watcher.Run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
