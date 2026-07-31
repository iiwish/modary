package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"example.com/modary-counter-consumer/internal/project"
	"github.com/iiwish/modary/appcmd"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := appcmd.Run(ctx, os.Args[1:], project.Definition, project.CommandOptions()); err != nil &&
		!errors.Is(err, context.Canceled) {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
