package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryUsesApacheLicenseAndAggregatedNotice(t *testing.T) {
	root := filepath.Dir(repositoryMakefile(t))
	license, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"Apache License",
		"Version 2.0, January 2004",
		"3. Grant of Patent License.",
		"END OF TERMS AND CONDITIONS",
	} {
		if !strings.Contains(string(license), marker) {
			t.Errorf("LICENSE does not contain %q", marker)
		}
	}

	notice, err := os.ReadFile(filepath.Join(root, "NOTICE"))
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"Modary", "xeipuuv", "MongoDB, Inc."} {
		if !strings.Contains(string(notice), marker) {
			t.Errorf("NOTICE does not contain %q", marker)
		}
	}
}

const testReleaseVersion = "v0.1.0-alpha.1"

func TestReleasePreflightAcceptsCompleteCandidateAndExactTag(t *testing.T) {
	repository := newReleaseFixture(t)
	if output, err := runReleasePreflight(t, repository, testReleaseVersion, "candidate"); err != nil {
		t.Fatalf("complete candidate failed: %v\n%s", err, output)
	}
	finalizeReleaseChangelog(t, repository)
	runGitFixture(t, repository, "tag", "-a", testReleaseVersion, "-m", testReleaseVersion)
	if output, err := runReleasePreflight(t, repository, testReleaseVersion, "tag"); err != nil {
		t.Fatalf("exact tag failed: %v\n%s", err, output)
	}
}

func TestReleasePreflightRejectsInvalidVersion(t *testing.T) {
	repository := newReleaseFixture(t)
	for _, version := range []string{"v0.1.0", "v0.1.0-alpha.01"} {
		output, err := runReleasePreflight(t, repository, version, "candidate")
		if err == nil || !strings.Contains(output, "semantic") {
			t.Fatalf("invalid version %s check = %v, output=%q", version, err, output)
		}
	}
}

func TestReleasePreflightRejectsMissingOwnerInputs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
		want   string
	}{
		{
			name: "Go baseline",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				replaceDocsFixture(t, filepath.Join(repository, "go.mod"), "go 1.26.5", "go 1.26")
			},
			want: "security-patched 1.26.5",
		},
		{
			name: "license",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				if err := os.Remove(filepath.Join(repository, "LICENSE")); err != nil {
					t.Fatal(err)
				}
			},
			want: "owner-selected redistribution license",
		},
		{
			name: "security contact",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				replaceDocsFixture(t, filepath.Join(repository, "SECURITY.md"),
					"- Private reporting channel: https://github.com/iiwish/modary/security/advisories/new",
					"- Private reporting channel: Pending owner selection")
			},
			want: "private security reporting channel",
		},
		{
			name: "invalid security contact",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				replaceDocsFixture(t, filepath.Join(repository, "SECURITY.md"),
					"- Private reporting channel: https://github.com/iiwish/modary/security/advisories/new",
					"- Private reporting channel: ask the maintainer")
			},
			want: "private security reporting channel",
		},
		{
			name: "origin",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				runGitFixture(t, repository, "remote", "set-url", "origin", "https://example.invalid/not-modary.git")
			},
			want: "canonical origin",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := newReleaseFixture(t)
			test.mutate(t, repository)
			output, err := runReleasePreflight(t, repository, testReleaseVersion, "candidate")
			if err == nil || !strings.Contains(output, test.want) {
				t.Fatalf("preflight = %v, output=%q, want %q", err, output, test.want)
			}
		})
	}
}

func TestReleasePreflightRejectsDirtyStateAndMissingTag(t *testing.T) {
	repository := newReleaseFixture(t)
	appendDocsFixture(t, filepath.Join(repository, "CHANGELOG.md"), "\nlocal mutation\n")
	output, err := runReleasePreflight(t, repository, testReleaseVersion, "candidate")
	if err == nil || !strings.Contains(output, "clean committed worktree") {
		t.Fatalf("dirty state check = %v, output=%q", err, output)
	}

	repository = newReleaseFixture(t)
	finalizeReleaseChangelog(t, repository)
	output, err = runReleasePreflight(t, repository, testReleaseVersion, "tag")
	if err == nil || !strings.Contains(output, "exact release tag") {
		t.Fatalf("missing tag check = %v, output=%q", err, output)
	}
}

func finalizeReleaseChangelog(t *testing.T, repository string) {
	t.Helper()
	replaceDocsFixture(t, filepath.Join(repository, "CHANGELOG.md"),
		"## "+testReleaseVersion+" - Unreleased",
		"## "+testReleaseVersion+" - 2026-07-31")
	runGitFixture(t, repository, "add", "CHANGELOG.md")
	runGitFixture(t, repository, "-c", "user.name=Modary Test", "-c", "user.email=modary@example.invalid", "commit", "--quiet", "-m", "finalize changelog")
}

func TestRemoteConsumerRemovesReplacementAndRunsCompleteGate(t *testing.T) {
	root, fakeGo, logFile := newRemoteConsumerFixture(t)
	output, err := runRemoteConsumer(t, root, fakeGo, logFile, false)
	if err != nil {
		t.Fatalf("remote consumer failed: %v\n%s", err, output)
	}
	data, readErr := os.ReadFile(logFile)
	if readErr != nil {
		t.Fatal(readErr)
	}
	log := string(data)
	if !strings.Contains(log, "|on|local|off|off||") || !strings.Contains(log, "/no-node:") {
		t.Fatalf("remote consumer did not pin the Go environment or Node-family shims:\n%s", log)
	}
	if !strings.Contains(log, "|1|test -count=1 ./...") {
		t.Fatalf("remote consumer did not identify the copied-out test context:\n%s", log)
	}
	for _, required := range []string{
		"mod edit -dropreplace=github.com/iiwish/modary",
		"mod edit -require=github.com/iiwish/modary@" + testReleaseVersion,
		"mod tidy",
		"run ./tools/modary verify",
		"run ./tools/modary generate --check",
		"run ./tools/modary check",
		"test -count=1 ./...",
		"build ./...",
		"run ./cmd/counter-console version",
	} {
		if !strings.Contains(log, required) {
			t.Errorf("remote consumer log does not contain %q:\n%s", required, log)
		}
	}
	sourceMod, readErr := os.ReadFile(filepath.Join(root, "examples", "counter", "go.mod"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(sourceMod), "replace github.com/iiwish/modary => ../..") {
		t.Fatal("remote gate modified the source-checkout consumer")
	}
}

func TestRemoteConsumerFailsWhenResolutionUsesReplacement(t *testing.T) {
	root, fakeGo, logFile := newRemoteConsumerFixture(t)
	output, err := runRemoteConsumer(t, root, fakeGo, logFile, true)
	if err == nil || !strings.Contains(output, "resolved through a replacement") {
		t.Fatalf("replacement check = %v, output=%q", err, output)
	}
}

func newReleaseFixture(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	writeDocsFixtureFile(t, filepath.Join(repository, "go.mod"), "module github.com/iiwish/modary\n\ngo 1.26.5\n")
	writeDocsFixtureFile(t, filepath.Join(repository, "LICENSE"), "owner-selected license text\n")
	writeDocsFixtureFile(t, filepath.Join(repository, "SECURITY.md"), "# Security\n\n- Private reporting channel: https://github.com/iiwish/modary/security/advisories/new\n")
	writeDocsFixtureFile(t, filepath.Join(repository, "CHANGELOG.md"), "# Changelog\n\n## "+testReleaseVersion+" - Unreleased\n")
	writeDocsFixtureFile(t, filepath.Join(repository, "docs", "f0-acceptance-report.md"), "# Acceptance\n\n- Status: Accepted\n")
	runGitFixture(t, repository, "init", "--quiet")
	runGitFixture(t, repository, "config", "user.name", "Modary Test")
	runGitFixture(t, repository, "config", "user.email", "modary@example.invalid")
	runGitFixture(t, repository, "add", "--all")
	runGitFixture(t, repository, "-c", "user.name=Modary Test", "-c", "user.email=modary@example.invalid", "commit", "--quiet", "-m", "candidate")
	runGitFixture(t, repository, "remote", "add", "origin", "https://github.com/iiwish/modary.git")
	return repository
}

func runReleasePreflight(t *testing.T, repository, version, mode string) (string, error) {
	t.Helper()
	root := filepath.Dir(repositoryMakefile(t))
	command := exec.Command("sh", filepath.Join(root, "scripts", "release-preflight.sh"), version, mode, repository)
	output, err := command.CombinedOutput()
	return string(output), err
}

func runGitFixture(t *testing.T, repository string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func newRemoteConsumerFixture(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	consumer := filepath.Join(root, "examples", "counter")
	writeDocsFixtureFile(t, filepath.Join(consumer, "go.mod"), `module example.com/consumer

go 1.26

require github.com/iiwish/modary v0.0.0

replace github.com/iiwish/modary => ../..
`)
	for _, relative := range []string{"tools/modary/main.go", "cmd/counter-console/main.go", "modary.yaml"} {
		writeDocsFixtureFile(t, filepath.Join(consumer, relative), "fixture\n")
	}
	logFile := filepath.Join(root, "go.log")
	fakeGo := filepath.Join(root, "fake-go")
	writeDocsFixtureFile(t, fakeGo, `#!/bin/sh
set -eu
printf '%s|%s|%s|%s|%s|%s|%s|%s|%s\n' "$PWD" "${GO111MODULE-}" "${GOTOOLCHAIN-}" "${GOENV-}" "${GOWORK-}" "${GOFLAGS-}" "$PATH" "${MODARY_EXTERNAL_CONSUMER_COPIED_OUT-}" "$*" >>"${MODARY_FAKE_GO_LOG}"
case "$*" in
  'mod edit -dropreplace=github.com/iiwish/modary')
    awk '!/^replace github.com\/iiwish\/modary /' go.mod >go.mod.next
    mv go.mod.next go.mod
    ;;
  'mod edit -require=github.com/iiwish/modary@'*)
    version=${1#mod}
    version=${MODARY_FAKE_VERSION}
    awk -v version="$version" '{
      if ($1 == "require" && $2 == "github.com/iiwish/modary") {
        print "require github.com/iiwish/modary " version
      } else {
        print
      }
    }' go.mod >go.mod.next
    mv go.mod.next go.mod
    ;;
  'list -m -f {{.Version}} github.com/iiwish/modary')
    printf '%s\n' "${MODARY_FAKE_VERSION}"
    ;;
  'list -m -f {{if .Replace}}{{.Replace.Path}}{{end}} github.com/iiwish/modary')
    printf '%s\n' "${MODARY_FAKE_REPLACED-}"
    ;;
esac
`)
	if err := os.Chmod(fakeGo, 0o755); err != nil {
		t.Fatal(err)
	}
	return root, fakeGo, logFile
}

func runRemoteConsumer(t *testing.T, root, fakeGo, logFile string, replaced bool) (string, error) {
	t.Helper()
	repositoryRoot := filepath.Dir(repositoryMakefile(t))
	command := exec.Command("sh", filepath.Join(repositoryRoot, "scripts", "remote-consumer.sh"), testReleaseVersion, root)
	replacement := ""
	if replaced {
		replacement = "/tmp/local-modary"
	}
	command.Env = append(os.Environ(),
		"GO="+fakeGo,
		"MODARY_FAKE_GO_LOG="+logFile,
		"MODARY_FAKE_VERSION="+testReleaseVersion,
		"MODARY_FAKE_REPLACED="+replacement,
	)
	output, err := command.CombinedOutput()
	return string(output), err
}
