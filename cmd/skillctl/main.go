package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"skillctl/internal/cli"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Execute prints the human (or --json) error report itself; main only
	// maps the failure onto the stable exit code.
	if err := cli.Execute(ctx); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}
