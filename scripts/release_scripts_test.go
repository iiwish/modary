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
	tagReleaseTrain(t, repository, testReleaseVersion)
	if output, err := runReleasePreflight(t, repository, testReleaseVersion, "tag"); err != nil {
		t.Fatalf("exact tag failed: %v\n%s", err, output)
	}
}

func TestReleasePreflightRejectsIncompleteComponentTagTrain(t *testing.T) {
	repository := newReleaseFixture(t)
	finalizeReleaseChangelog(t, repository)
	runGitFixture(t, repository, "tag", "-a", testReleaseVersion, "-m", testReleaseVersion)
	output, err := runReleasePreflight(t, repository, testReleaseVersion, "tag")
	if err == nil || !strings.Contains(output, "component release tag") {
		t.Fatalf("incomplete component tag train = %v, output=%q", err, output)
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

func TestReleasePreflightRejectsComponentVersionDrift(t *testing.T) {
	repository := newReleaseFixture(t)
	replaceDocsFixture(t, filepath.Join(repository, "components", "postgres", "go.mod"), testReleaseVersion, "v0.1.0-alpha.2")
	output, err := runReleasePreflight(t, repository, testReleaseVersion, "candidate")
	if err == nil || !strings.Contains(output, "must require github.com/iiwish/modary") {
		t.Fatalf("component version drift = %v, output=%q", err, output)
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
	refreshAcceptanceDigest(t, repository)
	runGitFixture(t, repository, "add", "CHANGELOG.md", ".ai-platform/evidence/T041/summary.md")
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
		"mod edit -dropreplace=github.com/iiwish/modary/components/postgres",
		"mod edit -dropreplace=github.com/iiwish/modary/components/governedpostgres",
		"mod edit -require=github.com/iiwish/modary@" + testReleaseVersion,
		"mod edit -require=github.com/iiwish/modary/components/postgres@" + testReleaseVersion,
		"mod edit -require=github.com/iiwish/modary/components/governedpostgres@" + testReleaseVersion,
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
	writeDocsFixtureFile(t, filepath.Join(repository, "docs", "f0-acceptance-report.md"), "# Acceptance\n\n- Status: Accepted\n- Current closure spec: `.ai-platform/specs/009-component-boundary-closure/spec.md`\n- Current closure evidence: `.ai-platform/evidence/T041/`\n")
	writeDocsFixtureFile(t, filepath.Join(repository, ".ai-platform", "evidence", "T041", "summary.md"), "# T041\n\n- Source digest: git-hash:0000000000000000000000000000000000000000\n")
	writeDocsFixtureFile(t, filepath.Join(repository, "components", "postgres", "go.mod"), "module github.com/iiwish/modary/components/postgres\n\ngo 1.26.5\n\nrequire github.com/iiwish/modary "+testReleaseVersion+"\n")
	writeDocsFixtureFile(t, filepath.Join(repository, "components", "governedpostgres", "go.mod"), "module github.com/iiwish/modary/components/governedpostgres\n\ngo 1.26.5\n\nrequire github.com/iiwish/modary "+testReleaseVersion+"\n")
	runGitFixture(t, repository, "init", "--quiet")
	refreshAcceptanceDigest(t, repository)
	runGitFixture(t, repository, "config", "user.name", "Modary Test")
	runGitFixture(t, repository, "config", "user.email", "modary@example.invalid")
	runGitFixture(t, repository, "add", "--all")
	runGitFixture(t, repository, "-c", "user.name=Modary Test", "-c", "user.email=modary@example.invalid", "commit", "--quiet", "-m", "candidate")
	runGitFixture(t, repository, "remote", "add", "origin", "https://github.com/iiwish/modary.git")
	return repository
}

func refreshAcceptanceDigest(t *testing.T, repository string) {
	t.Helper()
	command := exec.Command(filepath.Join(filepath.Dir(repositoryMakefile(t)), "scripts", "acceptance-source-digest.sh"), repository)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("refresh acceptance digest: %v\n%s", err, output)
	}
	path := filepath.Join(repository, ".ai-platform", "evidence", "T041", "summary.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	start := strings.Index(text, "- Source digest: ")
	if start < 0 {
		t.Fatalf("acceptance summary has no source digest: %s", text)
	}
	end := strings.IndexByte(text[start:], '\n')
	if end < 0 {
		end = len(text) - start
	}
	line := "- Source digest: " + strings.TrimSpace(string(output))
	text = text[:start] + line + text[start+end:]
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func tagReleaseTrain(t *testing.T, repository, version string) {
	t.Helper()
	for _, tag := range []string{
		version,
		"components/postgres/" + version,
		"components/governedpostgres/" + version,
	} {
		runGitFixture(t, repository, "tag", "-a", tag, "-m", tag)
	}
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

require (
	github.com/iiwish/modary/components/postgres v0.0.0
	github.com/iiwish/modary/components/governedpostgres v0.0.0
)

replace github.com/iiwish/modary => ../..

replace github.com/iiwish/modary/components/postgres => ../../components/postgres

replace github.com/iiwish/modary/components/governedpostgres => ../../components/governedpostgres
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
  'list -m -f {{.Version}} '*)
    printf '%s\n' "${MODARY_FAKE_VERSION}"
    ;;
  'list -m -f {{if .Replace}}{{.Replace.Path}}{{end}} '*)
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
