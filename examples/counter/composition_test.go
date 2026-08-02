package consumer_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"example.com/modary-counter-consumer/internal/project"
	"github.com/iiwish/modary/appcmd"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/projecttool"
)

var sharedDefinitionProvider appkit.DefinitionProvider = project.Definition

var (
	_ appkit.DefinitionProvider      = project.Definition
	_ appcmd.DefinitionProvider      = sharedDefinitionProvider
	_ projecttool.DefinitionProvider = sharedDefinitionProvider
)

func TestApplicationCommandsDeferCompositionUntilRuntimeCommands(t *testing.T) {
	var calls atomic.Int64
	provider := func() (appkit.Definition, error) {
		calls.Add(1)
		panic("pure command invoked the composition provider")
	}
	options := project.CommandOptions()
	options.Stdout = io.Discard
	options.Stderr = io.Discard

	for _, args := range [][]string{
		nil,
		{"help"},
		{"version"},
		{"serve", "--help"},
		{"action", "--help"},
		{"unknown"},
		{"version", "extra"},
	} {
		_ = appcmd.Run(context.Background(), args, provider, options)
	}
	if calls.Load() != 0 {
		t.Fatalf("pure commands constructed Definition %d times", calls.Load())
	}
}

func TestApplicationCommandPreservesCompositionErrors(t *testing.T) {
	options := project.CommandOptions()
	options.Stdout = io.Discard
	options.Stderr = io.Discard

	t.Run("error identity", func(t *testing.T) {
		cause := errors.New("adapter configuration failed")
		err := appcmd.Run(context.Background(), []string{"serve"}, func() (appkit.Definition, error) {
			return appkit.Definition{}, cause
		}, options)
		if !errors.Is(err, cause) {
			t.Fatalf("appcmd.Run() error = %v, want composition cause", err)
		}
		if text := err.Error(); !strings.Contains(text, "construct application Definition") ||
			!strings.Contains(text, cause.Error()) {
			t.Fatalf("composition diagnostic = %q", text)
		}
	})

	t.Run("adapter diagnostic", func(t *testing.T) {
		err := appcmd.Run(context.Background(), []string{"serve"}, func() (appkit.Definition, error) {
			return project.NewDefinition(project.Config{})
		}, options)
		if err == nil {
			t.Fatal("appcmd.Run() accepted an invalid adapter configuration")
		}
		if text := err.Error(); !strings.Contains(text, "construct application Definition") ||
			!strings.Contains(text, "configure PostgreSQL") ||
			!strings.Contains(text, "PostgreSQL URL") {
			t.Fatalf("adapter diagnostic = %q", text)
		}
	})
}

func mustDefinition(t *testing.T, config project.Config) appkit.Definition {
	t.Helper()
	definition, err := project.NewDefinition(config)
	if err != nil {
		t.Fatalf("NewDefinition() error = %v", err)
	}
	return definition
}
