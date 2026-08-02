package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCommandBuilds(t *testing.T) {
	command := exec.Command("go", "build", "-o", filepath.Join(t.TempDir(), "modary"), ".")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("go build .: %v\n%s", err, output)
	}
}

func TestCommandCreatesProjectWithDevelopmentOverride(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "command-api")
	command := exec.Command("go", "run", ".", "new", destination, "--profile", "api", "--module", "example.com/command-api")
	command.Env = append(os.Environ(),
		"MODARY_STARTER_VERSION=v0.1.0-alpha.3",
		"MODARY_STARTER_REPLACE="+commandRepositoryRoot(t),
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("modary new: %v\n%s", err, output)
	}
	module, err := os.ReadFile(filepath.Join(destination, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(module), "module example.com/command-api") ||
		!strings.Contains(string(module), "replace github.com/iiwish/modary") {
		t.Fatalf("generated go.mod:\n%s", module)
	}
}

func commandRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate command test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
