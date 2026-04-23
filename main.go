package main

import (
	"context"
	"os/signal"
	"syscall"

	"haruki-cloud/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	server.Run(ctx)
}
