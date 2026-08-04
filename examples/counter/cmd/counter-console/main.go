package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"example.com/modary-counter-consumer/internal/project"
	"github.com/iiwish/modary/appcmd"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	options, err := project.CommandOptions()
	if err == nil {
		options.Logger = logger
		err = appcmd.Run(ctx, os.Args[1:], project.Definition, options)
	}
	if err != nil &&
		!errors.Is(err, context.Canceled) {
		logger.Error("process failed", "event", "process.failed")
		os.Exit(1)
	}
}
