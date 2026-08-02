package scripts_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const neutralityCheckTestTimeout = 30 * time.Second

func TestCheckNeutralityAcceptsIndependentConsumerAndContributorDocs(t *testing.T) {
	repository := newNeutralityRepository(t)

	output, err := runNeutralityCheck(t, repository, "")
	if err != nil {
		t.Fatalf("clean neutrality check failed: %v\n%s", err, output)
	}
}

func TestCheckNeutralityRejectsDynamicProductionLeaks(t *testing.T) {
	for _, test := range []struct {
		name    string
		prepare func(*testing.T, string)
		want    string
	}{
		{
			name: "unexpected private framework tree",
			prepare: func(t *testing.T, repository string) {
				writeNeutralityFile(t, filepath.Join(repository, "internal", "unexpected", "unexpected.go"), "package unexpected\n", 0o644)
			},
			want: "unexpected private framework tree",
		},
		{
			name: "unexpected top-level consumer domain",
			prepare: func(t *testing.T, repository string) {
				writeNeutralityFile(t, filepath.Join(repository, ".ignore"), "surprise/**\n", 0o644)
				writeNeutralityFile(t, filepath.Join(repository, "surprise", "domain.go"), "package surprise\n\nconst product = \"Rulary\"\n", 0o644)
			},
			want: "consumer-domain terms remain",
		},
		{
			name: "consumer domain in authoritative spec",
			prepare: func(t *testing.T, repository string) {
				writeNeutralityFile(t, filepath.Join(repository, ".ai-platform", "specs", "002-framework-decoupling", "spec.md"), "# Spec\n\nRulary application contract.\n", 0o644)
			},
			want: "consumer-domain terms remain",
		},
		{
			name: "consumer domain in authoritative plan",
			prepare: func(t *testing.T, repository string) {
				writeNeutralityFile(t, filepath.Join(repository, ".ai-platform", "specs", "002-framework-decoupling", "plan.md"), "# Plan\n\nPreserve the downstream Ruleset.\n", 0o644)
			},
			want: "consumer-domain terms remain",
		},
		{
			name: "machine path in authoritative spec",
			prepare: func(t *testing.T, repository string) {
				writeNeutralityFile(t, filepath.Join(repository, ".ai-platform", "specs", "002-framework-decoupling", "spec.md"), "# Spec\n\nConsumer root: /Users/example/product.\n", 0o644)
			},
			want: "machine-specific absolute path remains",
		},
		{
			name: "machine path in authoritative plan",
			prepare: func(t *testing.T, repository string) {
				writeNeutralityFile(t, filepath.Join(repository, ".ai-platform", "specs", "002-framework-decoupling", "plan.md"), "# Plan\n\nConsumer root: /home/example/product.\n", 0o644)
			},
			want: "machine-specific absolute path remains",
		},
		{
			name: "unexpected top-level legacy import",
			prepare: func(t *testing.T, repository string) {
				writeNeutralityFile(t, filepath.Join(repository, "surprise", "legacy.go"), "package surprise\n\nimport _ \"github.com/iiwish/modary/core/action\"\n", 0o644)
			},
			want: "removed application-owned import path",
		},
		{
			name: "unexpected top-level executable",
			prepare: func(t *testing.T, repository string) {
				writeNeutralityFile(t, filepath.Join(repository, "surprise", "main.go"), "package main\n\nfunc main() {}\n", 0o644)
			},
			want: "contains an application executable",
		},
		{
			name: "consumer identifier in framework production",
			prepare: func(t *testing.T, repository string) {
				writeNeutralityFile(t, filepath.Join(repository, "surprise", "consumer.go"), "package surprise\n\nconst actionID = \"counter.increment\"\n", 0o644)
			},
			want: "leaked into framework production code",
		},
		{
			name: "missing conformance consumer",
			prepare: func(t *testing.T, repository string) {
				if err := os.RemoveAll(filepath.Join(repository, "examples", "counter")); err != nil {
					t.Fatal(err)
				}
			},
			want: "external consumer conformance module is missing",
		},
		{
			name: "consumer shares framework module",
			prepare: func(t *testing.T, repository string) {
				writeNeutralityFile(t, filepath.Join(repository, "examples", "counter", "go.mod"), "module github.com/iiwish/modary\n\ngo 1.26\n", 0o644)
			},
			want: "module distinct from the framework",
		},
		{
			name: "consumer imports private framework package",
			prepare: func(t *testing.T, repository string) {
				writeNeutralityFile(t, filepath.Join(repository, "examples", "counter", "consumer.go"), "package consumer\n\nimport _ \"github.com/iiwish/modary/internal/actionruntime\"\n", 0o644)
			},
			want: "imports a private Modary package",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := newNeutralityRepository(t)
			test.prepare(t, repository)

			output, err := runNeutralityCheck(t, repository, "")
			if err == nil || !strings.Contains(output, test.want) {
				t.Fatalf("neutrality check = %v, output=%q, want %q", err, output, test.want)
			}
		})
	}
}

func TestCheckNeutralityFailsClosedWhenScannerFails(t *testing.T) {
	for _, tool := range []string{"rg", "find"} {
		tool := tool
		t.Run(tool, func(t *testing.T) {
			repository := newNeutralityRepository(t)
			fakeBin := t.TempDir()
			writeNeutralityFile(
				t,
				filepath.Join(fakeBin, tool),
				"#!/bin/sh\nprintf 'simulated "+tool+" input failure\\n' >&2\nexit 2\n",
				0o755,
			)

			output, err := runNeutralityCheck(t, repository, fakeBin)
			if err == nil || !strings.Contains(output, "simulated "+tool+" input failure") {
				t.Fatalf("neutrality check = %v, output=%q, want fail-closed %s error", err, output, tool)
			}
		})
	}
}

func newNeutralityRepository(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("neutrality gate requires a POSIX shell")
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	scriptSource := filepath.Join(workingDirectory, "check-neutrality.sh")
	script, err := os.ReadFile(scriptSource)
	if err != nil {
		t.Fatal(err)
	}

	repository := t.TempDir()
	writeNeutralityFile(t, filepath.Join(repository, "scripts", "check-neutrality.sh"), string(script), 0o755)
	writeNeutralityFile(t, filepath.Join(repository, "go.mod"), "module github.com/iiwish/modary\n\ngo 1.26\n", 0o644)
	writeNeutralityFile(t, filepath.Join(repository, "framework.go"), "package modary\n", 0o644)
	writeNeutralityFile(t, filepath.Join(repository, "cmd", "modary", "main.go"), "package main\n\nfunc main() {}\n", 0o644)
	for _, packageName := range []string{
		"actionruntime", "callbackcontract", "databasecontrol", "filepolicy", "jsonschema", "jsonvalue", "moduleassembly", "quality",
		"runtimecontrol", "safeerr", "sqlpolicy", "testsupport", "transactionoutcome",
	} {
		writeNeutralityFile(
			t,
			filepath.Join(repository, "internal", packageName, "doc.go"),
			"package "+packageName+"\n",
			0o644,
		)
	}
	writeNeutralityFile(
		t,
		filepath.Join(repository, "README.md"),
		"# Framework\n\nConformance: `GOWORK=off go run ./cmd/counter-console version`.\n",
		0o644,
	)
	writeNeutralityFile(t, filepath.Join(repository, ".ai-platform", "specs", "002-framework-decoupling", "spec.md"), "# Framework Spec\n\nConsumer-neutral contract.\n", 0o644)
	writeNeutralityFile(t, filepath.Join(repository, ".ai-platform", "specs", "002-framework-decoupling", "plan.md"), "# Framework Plan\n\nConsumer-neutral implementation.\n", 0o644)
	writeNeutralityFile(t, filepath.Join(repository, ".ai-platform", "specs", "002-framework-decoupling", "analysis.md"), "# Historical Analysis\n\nRulary at /Users/contributor/downstream.\n", 0o644)
	writeNeutralityFile(t, filepath.Join(repository, ".ai-platform", "specs", "002-framework-decoupling", "tasks.md"), "# Historical Tasks\n\nRulary workspace.\n", 0o644)
	writeNeutralityFile(t, filepath.Join(repository, ".ai-platform", "specs", "002-framework-decoupling", "packets", "T010.yaml"), "historical: /home/contributor/rulary\n", 0o644)
	writeNeutralityFile(t, filepath.Join(repository, ".ai-platform", "evidence", "T010", "summary.md"), "# Historical Evidence\n\nRulary at /Users/contributor/downstream.\n", 0o644)
	writeNeutralityFile(t, filepath.Join(repository, "neutral path", "empty file.txt"), "", 0o644)
	writeNeutralityFile(t, filepath.Join(repository, "neutral path", "line\nbreak.txt"), "neutral\n", 0o644)
	writeNeutralityFile(
		t,
		filepath.Join(repository, "examples", "counter", "go.mod"),
		"module example.com/modary-consumer\n\ngo 1.26\n\nrequire github.com/iiwish/modary v0.0.0\n\nreplace github.com/iiwish/modary => ../..\n",
		0o644,
	)
	writeNeutralityFile(t, filepath.Join(repository, "examples", "counter", "consumer.go"), "package consumer\n\nimport _ \"github.com/iiwish/modary/action\"\n", 0o644)

	for _, arguments := range [][]string{
		{"init", "-q"},
		{"config", "user.name", "Modary Test"},
		{"config", "user.email", "modary@example.invalid"},
		{"add", "-A"},
		{"commit", "-qm", "neutral fixture"},
	} {
		runNeutralityGit(t, repository, arguments...)
	}
	return repository
}

func runNeutralityCheck(t *testing.T, repository, pathPrefix string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), neutralityCheckTestTimeout)
	defer cancel()

	command := exec.CommandContext(ctx, "sh", filepath.Join("scripts", "check-neutrality.sh"))
	command.Dir = repository
	if pathPrefix != "" {
		path := pathPrefix + string(os.PathListSeparator) + os.Getenv("PATH")
		command.Env = replaceNeutralityEnvironment(os.Environ(), "PATH", path)
	}
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("neutrality check did not terminate: %v", ctx.Err())
	}
	return string(output), err
}

func replaceNeutralityEnvironment(environment []string, key, value string) []string {
	prefix := key + "="
	replaced := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			replaced = append(replaced, entry)
		}
	}
	return append(replaced, prefix+value)
}

func runNeutralityGit(t *testing.T, repository string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
}

func writeNeutralityFile(t *testing.T, name, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}
