package starter_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iiwish/modary/starter"
)

func TestRunCreatesAPIProfileWithFlagsAfterDestination(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "cli-api")
	var stdout bytes.Buffer
	err := starter.Run(context.Background(), []string{
		"new", destination,
		"--profile", "api",
		"--module", "example.com/acme/cli-api",
		"--name", "CLI API",
	}, starter.Options{
		Stdout:        &stdout,
		ModaryVersion: "v0.1.0-alpha.3",
		ModaryReplace: repositoryRoot(t),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "api") || !strings.Contains(stdout.String(), destination) {
		t.Fatalf("Run() output = %q", stdout.String())
	}
	runGo(t, destination, "mod", "tidy")
	runGo(t, destination, "test", "./...")
}

func TestRunValidatesSyntaxBeforeCreation(t *testing.T) {
	tests := [][]string{
		{"new"},
		{"new", "one", "two"},
		{"new", "example", "--profile"},
		{"new", "example", "--profile", "api", "--profile", "api"},
		{"new", "example", "--unknown", "value"},
		{"unknown"},
	}
	for _, args := range tests {
		if err := starter.Run(context.Background(), args, starter.Options{Stdout: &bytes.Buffer{}}); !errors.Is(err, starter.ErrUsage) {
			t.Errorf("Run(%q) error = %v", args, err)
		}
	}
}

func TestRunAcceptsRepeatableAdminComponentSelection(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "cli-admin")
	var stdout bytes.Buffer
	err := starter.Run(context.Background(), []string{
		"new", destination, "--profile", "admin", "--with", "tasks", "--with=audit",
		"--module", "example.com/acme/cli-admin",
	}, starter.Options{Stdout: &stdout, ModaryVersion: "v0.1.0-alpha.3", ModaryReplace: repositoryRoot(t)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"components":["audit","tasks"]`) {
		t.Fatalf("Run() output = %q", stdout.String())
	}
}

func TestRunHelpHasNoFilesystemSideEffects(t *testing.T) {
	var output bytes.Buffer
	if err := starter.Run(context.Background(), []string{"help"}, starter.Options{Stdout: &output}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "modary new") || !strings.Contains(output.String(), "--profile") ||
		!strings.Contains(output.String(), "api|admin|governed") {
		t.Fatalf("help output = %q", output.String())
	}
}
