// Command modary creates consumer-owned projects from first-party Profiles.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"

	"github.com/iiwish/modary/starter"
	"golang.org/x/mod/semver"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := starter.Run(ctx, os.Args[1:], commandOptions()); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		if errors.Is(err, starter.ErrUsage) {
			_, _ = fmt.Fprint(os.Stderr, starter.CommandUsage)
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func commandOptions() starter.Options {
	version := os.Getenv("MODARY_STARTER_VERSION")
	if version == "" {
		if info, ok := debug.ReadBuildInfo(); ok && semver.IsValid(info.Main.Version) {
			version = info.Main.Version
		}
	}
	return starter.Options{
		ModaryVersion: version,
		ModaryReplace: os.Getenv("MODARY_STARTER_REPLACE"),
	}
}
