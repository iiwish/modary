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

func TestCheckSourceDiffCoversTrackedAndNULSafeUntrackedPaths(t *testing.T) {
	t.Run("clean", func(t *testing.T) {
		repository, script := newDiffCheckRepository(t)
		if output, err := runDiffCheck(t, repository, script); err != nil {
			t.Fatalf("clean check failed: %v\n%s", err, output)
		}
	})

	for _, test := range []struct {
		name    string
		prepare func(*testing.T, string)
		want    string
	}{
		{
			name: "tracked whitespace",
			prepare: func(t *testing.T, repository string) {
				t.Helper()
				writeDiffCheckFile(t, filepath.Join(repository, "tracked.txt"), "changed  \n")
			},
			want: "trailing whitespace",
		},
		{
			name: "staged whitespace",
			prepare: func(t *testing.T, repository string) {
				t.Helper()
				writeDiffCheckFile(t, filepath.Join(repository, "tracked.txt"), "staged  \n")
				runDiffCheckGit(t, repository, "add", "tracked.txt")
			},
			want: "trailing whitespace",
		},
		{
			name: "committed whitespace",
			prepare: func(t *testing.T, repository string) {
				t.Helper()
				writeDiffCheckFile(t, filepath.Join(repository, "committed.txt"), "committed  \n")
				runDiffCheckGit(t, repository, "add", "committed.txt")
				runDiffCheckGit(t, repository, "commit", "-qm", "bad whitespace fixture")
			},
			want: "trailing whitespace",
		},
		{
			name: "untracked whitespace with spaces",
			prepare: func(t *testing.T, repository string) {
				t.Helper()
				writeDiffCheckFile(t, filepath.Join(repository, "untracked source with spaces.txt"), "bad  \n")
			},
			want: "trailing whitespace",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, script := newDiffCheckRepository(t)
			test.prepare(t, repository)
			output, err := runDiffCheck(t, repository, script)
			if err == nil || !strings.Contains(output, test.want) {
				t.Fatalf("check = %v, output=%q, want %q", err, output, test.want)
			}
		})
	}

	t.Run("evidence patch excluded", func(t *testing.T) {
		repository, script := newDiffCheckRepository(t)
		name := filepath.Join(repository, ".ai-platform", "evidence", "T999", "record.patch")
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		writeDiffCheckFile(t, name, "evidence whitespace  \n")
		if output, err := runDiffCheck(t, repository, script); err != nil {
			t.Fatalf("excluded evidence patch failed: %v\n%s", err, output)
		}
	})

	t.Run("pinned upstream fixture preserves bytes", func(t *testing.T) {
		repository, script := newDiffCheckRepository(t)
		name := filepath.Join(repository, "internal", "jsonschema", "testdata",
			"json-schema-test-suite", "draft7", "not.json")
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		writeDiffCheckFile(t, name, "{ \"upstream\": true }  \n")
		if output, err := runDiffCheck(t, repository, script); err != nil {
			t.Fatalf("pinned upstream fixture failed: %v\n%s", err, output)
		}
	})

	t.Run("whitespace attribute is narrowly scoped", func(t *testing.T) {
		repository, script := newDiffCheckRepository(t)
		name := filepath.Join(repository, "internal", "jsonschema", "testdata",
			"json-schema-test-suite", "draft6", "not.json")
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		writeDiffCheckFile(t, name, "{ \"not-pinned\": true }  \n")
		output, err := runDiffCheck(t, repository, script)
		if err == nil || !strings.Contains(output, "trailing whitespace") {
			t.Fatalf("check = %v, output=%q, want trailing whitespace", err, output)
		}
	})
}

func TestCheckSourceDiffRejectsUntrackedSpecialFilesWithoutReading(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic-link and FIFO setup requires POSIX filesystem semantics")
	}

	for _, pathname := range []string{
		"untracked-special",
		filepath.Join(".ai-platform", "evidence", "T999", "diff.patch"),
	} {
		pathname := pathname
		t.Run("symlink "+pathname, func(t *testing.T) {
			repository, script := newDiffCheckRepository(t)
			name := filepath.Join(repository, pathname)
			if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(repository, "tracked.txt"), name); err != nil {
				t.Fatal(err)
			}
			output, err := runDiffCheck(t, repository, script)
			if err == nil || !strings.Contains(output, "unsupported untracked path type") {
				t.Fatalf("symlink check = %v, output=%q", err, output)
			}
		})

		t.Run("fifo "+pathname, func(t *testing.T) {
			repository, script := newDiffCheckRepository(t)
			name := filepath.Join(repository, pathname)
			if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command("mkfifo", name).CombinedOutput(); err != nil {
				t.Fatalf("mkfifo: %v\n%s", err, output)
			}
			output, err := runDiffCheck(t, repository, script)
			if err == nil || !strings.Contains(output, "unsupported untracked path type") {
				t.Fatalf("FIFO check = %v, output=%q", err, output)
			}
		})
	}
}

func newDiffCheckRepository(t *testing.T) (string, string) {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(workingDirectory, "check-source-diff.sh")
	repository := t.TempDir()
	for _, arguments := range [][]string{
		{"init", "-q"},
		{"config", "user.name", "Modary Test"},
		{"config", "user.email", "modary@example.invalid"},
	} {
		runDiffCheckGit(t, repository, arguments...)
	}
	writeDiffCheckFile(t, filepath.Join(repository, "tracked.txt"), "clean\n")
	attributes, err := os.ReadFile(filepath.Join(workingDirectory, "..", ".gitattributes"))
	if err != nil {
		t.Fatal(err)
	}
	writeDiffCheckFile(t, filepath.Join(repository, ".gitattributes"), string(attributes))
	for _, arguments := range [][]string{{"add", "tracked.txt", ".gitattributes"}, {"commit", "-qm", "baseline"}} {
		runDiffCheckGit(t, repository, arguments...)
	}
	return repository, script
}

func runDiffCheckGit(t *testing.T, repository string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
}

func runDiffCheck(t *testing.T, repository, script string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "sh", script)
	command.Dir = repository
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("diff check read a blocking path: %v", ctx.Err())
	}
	return string(output), err
}

func writeDiffCheckFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
