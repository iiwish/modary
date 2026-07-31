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

const makeCommandTestTimeout = 30 * time.Second

func TestMakefilePinsEveryGoInvocation(t *testing.T) {
	makefile := repositoryMakefile(t)
	data, err := os.ReadFile(makefile)
	if err != nil {
		t.Fatal(err)
	}
	const pinned = "GO_COMMAND_ENV := GO111MODULE=on GOTOOLCHAIN=local GOENV=off GOWORK=off GOFLAGS="
	if !strings.Contains(string(data), pinned) {
		t.Fatalf("Makefile does not define the canonical Go command environment %q", pinned)
	}
	for lineNumber, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "$(GO)") && !strings.Contains(line, "$(GO_COMMAND_ENV)") {
			t.Errorf("Makefile:%d invokes $(GO) without $(GO_COMMAND_ENV): %s", lineNumber+1, line)
		}
	}
}

func TestMakeTestConsumerOverridesHostileGoEnvironmentAndExecutesGate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the F0 Make gate requires a POSIX shell")
	}

	repository := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repository, "testdata", "external-consumer"), 0o755); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(repository, "go-invocation.log")
	fakeGo := filepath.Join(repository, "fake-go")
	const fakeGoScript = `#!/bin/sh
set -eu
test "${GO111MODULE-}" = on
test "${GOTOOLCHAIN-}" = local
test "${GOENV-}" = off
test "${GOWORK-}" = off
test "${GOFLAGS+x}" = x
test -z "${GOFLAGS}"
test "${MODARY_EXTERNAL_CONSUMER_COPIED_OUT-}" = 0
printf '%s\n' "$@" >"${MODARY_FAKE_GO_LOG}"
`
	if err := os.WriteFile(fakeGo, []byte(fakeGoScript), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), makeCommandTestTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "make", "-f", repositoryMakefile(t), "test-consumer", "GO="+fakeGo)
	command.Dir = repository
	command.Env = replaceMakeEnvironment(os.Environ(),
		"GO111MODULE=off",
		"GOTOOLCHAIN=untrusted+auto",
		"GOENV="+filepath.Join(repository, "hostile-goenv"),
		"GOWORK="+filepath.Join(repository, "hostile.go.work"),
		"GOFLAGS=-run=^$ -count=0",
		"MODARY_EXTERNAL_CONSUMER_COPIED_OUT=1",
		"MODARY_FAKE_GO_LOG="+logFile,
	)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("make test-consumer did not terminate: %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf("make test-consumer failed under hostile Go environment: %v\n%s", err, output)
	}
	invocation, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("consumer Go gate was not invoked: %v", err)
	}
	if got, want := string(invocation), "test\n-count=1\n-v\n./...\n"; got != want {
		t.Fatalf("consumer Go invocation = %q, want %q", got, want)
	}
}

func TestMakeCrossBuildCompilesUnsupportedPlatformTestsOutsideRepository(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the F0 Make gate requires a POSIX shell")
	}

	repository := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repository, "testdata", "external-consumer"), 0o755); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(repository, "cross-invocations.log")
	fakeGo := filepath.Join(repository, "fake-go")
	const fakeGoScript = `#!/bin/sh
set -eu
test "${GO111MODULE-}" = on
test "${GOTOOLCHAIN-}" = local
test "${GOENV-}" = off
test "${GOWORK-}" = off
test "${GOFLAGS+x}" = x
test -z "${GOFLAGS}"
printf '%s\t%s\t%s\n' "${GOOS-}" "${GOARCH-}" "$*" >>"${MODARY_FAKE_GO_LOG}"
output=
while test "$#" -gt 0; do
	if test "$1" = -o; then
		shift
		output=$1
	fi
	shift
done
if test -n "$output"; then
	mkdir -p "$(dirname -- "$output")"
	: >"$output"
fi
`
	if err := os.WriteFile(fakeGo, []byte(fakeGoScript), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), makeCommandTestTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "make", "-f", repositoryMakefile(t), "cross-build", "GO="+fakeGo)
	command.Dir = repository
	command.Env = replaceMakeEnvironment(os.Environ(),
		"GOFLAGS=-run=^$",
		"MODARY_FAKE_GO_LOG="+logFile,
	)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("make cross-build did not terminate: %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf("make cross-build failed: %v\n%s", err, output)
	}
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read cross-build invocation log: %v", err)
	}
	builds := make(map[string]int)
	var compiledTests []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		platform := fields[0] + "/" + fields[1]
		if strings.Contains(line, "build ./...") {
			builds[platform]++
		}
		if strings.Contains(line, "test -c -o") {
			for index, field := range fields {
				if field == "-o" && index+1 < len(fields) {
					compiledTests = append(compiledTests, fields[index+1])
				}
			}
		}
	}
	for _, platform := range []string{
		"linux/amd64", "linux/arm64",
		"darwin/amd64", "darwin/arm64",
		"windows/amd64", "windows/arm64",
	} {
		if builds[platform] != 2 {
			t.Errorf("%s build invocations = %d, want framework and consumer; log:\n%s", platform, builds[platform], data)
		}
	}
	if len(compiledTests) != 8 {
		t.Fatalf("unsupported-platform test compile invocations = %d, want Windows appcmd/projecttool/sqlite and Darwin filepolicy for two architectures:\n%s", len(compiledTests), data)
	}
	for _, outputPath := range compiledTests {
		if !strings.HasPrefix(outputPath, "/tmp/modary-cross-tests.") {
			t.Errorf("cross-test output %q is not in the outside-repository temporary directory", outputPath)
		}
		if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
			t.Errorf("cross-test output %q was not removed, stat error = %v", outputPath, statErr)
		}
	}
	for _, packagePath := range []string{"./appcmd", "./projecttool", "./adapters/sqlite", "./internal/filepolicy"} {
		if !strings.Contains(string(data), packagePath) {
			t.Fatalf("cross-build did not compile %s tests:\n%s", packagePath, data)
		}
	}
}

func TestMakeCIComparesCompleteSourceStateAroundEveryGate(t *testing.T) {
	data, err := os.ReadFile(repositoryMakefile(t))
	if err != nil {
		t.Fatal(err)
	}
	makefile := string(data)
	start := strings.Index(makefile, "\nci:\n")
	if start < 0 {
		t.Fatal("Makefile has no ci recipe")
	}
	recipe := makefile[start:]
	for _, required := range []string{
		"./scripts/source-state.sh >\"$$before\"",
		"$(MAKE) ci-gates",
		"./scripts/source-state.sh >\"$$after\"",
		"cmp -s \"$$before\" \"$$after\"",
	} {
		if !strings.Contains(recipe, required) {
			t.Errorf("ci recipe does not contain %q", required)
		}
	}
}

func TestMakeRepeatTargetsOnlyStatefulHighRiskPackages(t *testing.T) {
	data, err := os.ReadFile(repositoryMakefile(t))
	if err != nil {
		t.Fatal(err)
	}
	makefile := string(data)
	packagesStart := strings.Index(makefile, "REPEAT_PACKAGES :=")
	if packagesStart < 0 {
		t.Fatal("Makefile has no bounded REPEAT_PACKAGES block")
	}
	packagesEnd := strings.Index(makefile[packagesStart:], "\n\n.PHONY:")
	if packagesEnd < 0 {
		t.Fatal("Makefile has no bounded REPEAT_PACKAGES block")
	}
	packages := makefile[packagesStart : packagesStart+packagesEnd]
	start := strings.Index(makefile, "\nrepeat:\n")
	end := strings.Index(makefile[start+1:], "\nfuzz-smoke:\n")
	if start < 0 || end < 0 {
		t.Fatal("Makefile has no bounded repeat recipe")
	}
	recipe := makefile[start : start+1+end]
	const frameworkRepeat = "$(GO_COMMAND_ENV) $(GO) test -shuffle=on -count=20 $(REPEAT_PACKAGES)"
	if strings.Count(recipe, frameworkRepeat) != 1 {
		t.Fatalf("framework repeat gate is not the curated package command:\n%s", recipe)
	}
	for _, line := range strings.Split(recipe, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "$(GO_COMMAND_ENV) $(GO) test") && strings.HasSuffix(line, "./...") {
			t.Fatal("framework repeat gate includes every static and governance package")
		}
	}
	for _, required := range []string{
		"REPEAT_PACKAGES",
		"-shuffle=on -count=20",
		"MODARY_EXTERNAL_CONSUMER_COPIED_OUT=1",
	} {
		if !strings.Contains(recipe, required) {
			t.Errorf("repeat recipe does not contain %q", required)
		}
	}
	for _, required := range []string{
		"./action", "./adapters/...", "./appcmd", "./appkit", "./database",
		"./internal/actionruntime", "./internal/callbackcontract", "./internal/databasecontrol", "./internal/filepolicy",
		"./internal/jsonschema/...", "./internal/jsonvalue", "./internal/runtimecontrol",
		"./internal/safeerr", "./internal/sqlpolicy", "./internal/transactionoutcome", "./module", "./projecttool",
		"./transport/httpapi",
	} {
		if !strings.Contains(packages, required) {
			t.Errorf("REPEAT_PACKAGES does not include %s", required)
		}
	}
}

func TestMakeFuzzSmokeCoversManifestJSONProtocolAndDarwinACLParsers(t *testing.T) {
	data, err := os.ReadFile(repositoryMakefile(t))
	if err != nil {
		t.Fatal(err)
	}
	makefile := string(data)
	for _, target := range []string{
		"FuzzParseManifestFailsClosed",
		"FuzzDecodeFailsClosed",
		"FuzzCompileAndValidateFlagFailsClosed",
		"FuzzProtocolJSONDecodersFailClosed",
		"FuzzParseExtendedSecurityResponse",
		"FuzzParseKauthFileSecurity",
	} {
		if count := strings.Count(makefile, "-fuzz="+target); count != 1 {
			t.Errorf("fuzz target %s invocation count = %d, want 1", target, count)
		}
	}
	if !strings.Contains(makefile, `$(GO) env GOOS)" = darwin`) {
		t.Error("Darwin-only ACL fuzzers are not guarded by the native GOOS")
	}
}

func TestCIIncludesNativeDarwinARM64ContractGate(t *testing.T) {
	workflow := filepath.Join(filepath.Dir(repositoryMakefile(t)), ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(workflow)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"darwin-arm64:",
		"runs-on: macos-15",
		"go env GOOS GOARCH GOVERSION CGO_ENABLED",
		`test "$(go env GOOS)/$(go env GOARCH)" = darwin/arm64`,
		"run: make native-platform",
		`test -z "$(git status --porcelain --untracked-files=all)"`,
	} {
		if !strings.Contains(string(data), required) {
			t.Errorf("Darwin CI job does not contain %q", required)
		}
	}
}

func TestSourceStateDetectsUntrackedContentAndExecutableMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the F0 source-state gate requires a POSIX shell")
	}
	repository := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repository, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(repositoryMakefile(t)), "scripts", "source-state.sh"))
	if err != nil {
		t.Fatal(err)
	}
	checker := filepath.Join(repository, "scripts", "source-state.sh")
	if err := os.WriteFile(checker, source, 0o755); err != nil {
		t.Fatal(err)
	}
	snapshotSource, err := os.ReadFile(filepath.Join(filepath.Dir(repositoryMakefile(t)), "scripts", "review-source-state.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "scripts", "review-source-state.sh"), snapshotSource, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = repository
		if output, commandErr := command.CombinedOutput(); commandErr != nil {
			t.Fatalf("git %v: %v\n%s", args, commandErr, output)
		}
	}
	runGit("init", "--quiet")
	runGit("-c", "user.name=Modary Test", "-c", "user.email=modary@example.invalid", "commit", "--quiet", "--allow-empty", "-m", "baseline")

	untracked := filepath.Join(repository, "consumer.go")
	if err := os.WriteFile(untracked, []byte("package consumer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		command := exec.Command("sh", checker)
		command.Dir = repository
		output, commandErr := command.CombinedOutput()
		if commandErr != nil {
			t.Fatalf("source-state: %v\n%s", commandErr, output)
		}
		return string(output)
	}
	regular := snapshot()
	if err := os.Chmod(untracked, 0o755); err != nil {
		t.Fatal(err)
	}
	executable := snapshot()
	if regular == executable || !strings.Contains(executable, "new file mode 100755") {
		t.Fatalf("source-state did not detect executable mode:\nregular=%s\nexecutable=%s", regular, executable)
	}
	if err := os.WriteFile(untracked, []byte("package changed\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if changed := snapshot(); changed == executable {
		t.Fatal("source-state did not detect untracked content change")
	}

	link := filepath.Join(repository, "source-link")
	if err := os.Symlink("target", link); err != nil {
		t.Fatal(err)
	}
	linkState := snapshot()
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target\n", link); err != nil {
		t.Fatal(err)
	}
	if trailingNewlineState := snapshot(); trailingNewlineState == linkState {
		t.Fatal("source-state did not detect a trailing newline in a symlink target")
	}

	newlinePath := filepath.Join(repository, "line\nbreak.txt")
	beforeNewlinePath := snapshot()
	if err := os.WriteFile(newlinePath, []byte("newline path\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if afterNewlinePath := snapshot(); afterNewlinePath == beforeNewlinePath {
		t.Fatal("source-state did not detect a newline-containing path")
	}

	evidence := filepath.Join(repository, ".ai-platform", "evidence", "T016", "review.md")
	if err := os.MkdirAll(filepath.Dir(evidence), 0o755); err != nil {
		t.Fatal(err)
	}
	beforeEvidence := snapshot()
	if err := os.WriteFile(evidence, []byte("review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if afterEvidence := snapshot(); afterEvidence == beforeEvidence {
		t.Fatal("complete CI source-state excluded T016 evidence")
	}

	beforeCommit := snapshot()
	runGit("add", "consumer.go")
	runGit("-c", "user.name=Modary Test", "-c", "user.email=modary@example.invalid", "commit", "--quiet", "-m", "source change")
	if afterCommit := snapshot(); afterCommit == beforeCommit || !strings.Contains(afterCommit, "head\t") {
		t.Fatal("source-state did not detect a source-changing commit")
	}
}

func TestT016ReviewStateIsContentAddressedAndExcludesOnlyOwnEvidence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the F0 review-state gate requires a POSIX shell")
	}
	repository := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repository, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(repositoryMakefile(t)), "scripts", "review-source-state.sh"))
	if err != nil {
		t.Fatal(err)
	}
	checker := filepath.Join(repository, "scripts", "review-source-state.sh")
	if err := os.WriteFile(checker, source, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = repository
		if output, commandErr := command.CombinedOutput(); commandErr != nil {
			t.Fatalf("git %v: %v\n%s", args, commandErr, output)
		}
	}
	runGit("init", "--quiet")
	tracked := filepath.Join(repository, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("tracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "--all")
	runGit("-c", "user.name=Modary Test", "-c", "user.email=modary@example.invalid", "commit", "--quiet", "-m", "baseline")

	snapshotWithTMPDIR := func(temporaryDirectory string) (string, error) {
		t.Helper()
		command := exec.Command("sh", checker, "--exclude-t016-evidence")
		command.Dir = repository
		if temporaryDirectory != "" {
			for _, variable := range os.Environ() {
				if !strings.HasPrefix(variable, "TMPDIR=") {
					command.Env = append(command.Env, variable)
				}
			}
			command.Env = append(command.Env, "TMPDIR="+temporaryDirectory)
		}
		output, commandErr := command.CombinedOutput()
		return string(output), commandErr
	}
	snapshot := func() (string, error) {
		return snapshotWithTMPDIR("")
	}
	clean, err := snapshot()
	if err != nil {
		t.Fatalf("clean review-state: %v\n%s", err, clean)
	}
	repositoryTemporaryDirectory := filepath.Join(repository, "caller-temporary")
	if err := os.Mkdir(repositoryTemporaryDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	withRepositoryTMPDIR, err := snapshotWithTMPDIR(repositoryTemporaryDirectory)
	if err != nil {
		t.Fatalf("review-state with repository TMPDIR: %v\n%s", err, withRepositoryTMPDIR)
	}
	if withRepositoryTMPDIR != clean {
		t.Fatal("T016 review state depends on a caller-selected repository TMPDIR")
	}
	indexBefore, err := os.ReadFile(filepath.Join(repository, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	countBefore := gitOutput(t, repository, "count-objects", "-v")

	evidence := filepath.Join(repository, ".ai-platform", "evidence", "T016", "review-1.md")
	if err := os.MkdirAll(filepath.Dir(evidence), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidence, []byte("first review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	afterEvidence, err := snapshot()
	if err != nil {
		t.Fatalf("review-state with evidence: %v\n%s", err, afterEvidence)
	}
	if afterEvidence != clean {
		t.Fatal("T016 review state included its own evidence")
	}
	if err := os.WriteFile(evidence, []byte("second review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	afterEvidenceEdit, err := snapshot()
	if err != nil {
		t.Fatalf("review-state after evidence edit: %v\n%s", err, afterEvidenceEdit)
	}
	if afterEvidenceEdit != clean {
		t.Fatal("T016 review source-state included its own evidence")
	}

	untracked := filepath.Join(repository, "consumer.go")
	if err := os.WriteFile(untracked, []byte("package consumer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	regular, err := snapshot()
	if err != nil {
		t.Fatalf("review-state with untracked file: %v\n%s", err, regular)
	}
	if regular == clean {
		t.Fatal("T016 review state did not detect an untracked source file")
	}
	if err := os.Chmod(untracked, 0o755); err != nil {
		t.Fatal(err)
	}
	executable, err := snapshot()
	if err != nil {
		t.Fatalf("review-state with executable file: %v\n%s", err, executable)
	}
	if executable == regular {
		t.Fatal("T016 review state did not detect executable mode")
	}
	runGit("add", "consumer.go")
	runGit("-c", "user.name=Modary Test", "-c", "user.email=modary@example.invalid", "commit", "--quiet", "-m", "same source state")
	afterCommit, err := snapshot()
	if err != nil {
		t.Fatalf("review-state after commit: %v\n%s", err, afterCommit)
	}
	if afterCommit != executable {
		t.Fatal("T016 review state changed when identical source content was committed")
	}
	if err := os.WriteFile(tracked, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := snapshot()
	if err != nil {
		t.Fatalf("review-state after tracked edit: %v\n%s", err, changed)
	}
	if changed == afterCommit {
		t.Fatal("T016 review state did not detect a tracked content change")
	}
	if err := os.Remove(tracked); err != nil {
		t.Fatal(err)
	}
	deleted, err := snapshot()
	if err != nil {
		t.Fatalf("review-state after tracked deletion: %v\n%s", err, deleted)
	}
	if deleted == changed {
		t.Fatal("T016 review state did not detect a tracked deletion")
	}

	link := filepath.Join(repository, "source-link")
	if err := os.Symlink("target", link); err != nil {
		t.Fatal(err)
	}
	firstLink, err := snapshot()
	if err != nil {
		t.Fatalf("review-state with symlink: %v\n%s", err, firstLink)
	}
	if firstLink == deleted {
		t.Fatal("T016 review state did not detect a symlink")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target\n", link); err != nil {
		t.Fatal(err)
	}
	secondLink, err := snapshot()
	if err != nil {
		t.Fatalf("review-state after symlink target change: %v\n%s", err, secondLink)
	}
	if secondLink == firstLink {
		t.Fatal("T016 review state did not detect a symlink target change")
	}

	newlinePath := filepath.Join(repository, "line\nbreak.txt")
	if err := os.WriteFile(newlinePath, []byte("newline path\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	newlineState, err := snapshot()
	if err != nil {
		t.Fatalf("review-state with newline path: %v\n%s", err, newlineState)
	}
	if newlineState == secondLink {
		t.Fatal("T016 review state did not detect a newline-containing path")
	}

	indexAfter, err := os.ReadFile(filepath.Join(repository, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	if string(indexAfter) != string(indexBefore) {
		// The explicit consumer commit legitimately changed the real index.
		// Capture it again and prove one isolated snapshot does not.
		indexBefore = indexAfter
	}
	countBefore = gitOutput(t, repository, "count-objects", "-v")
	if _, err := snapshot(); err != nil {
		t.Fatal(err)
	}
	indexAfter, err = os.ReadFile(filepath.Join(repository, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	if string(indexAfter) != string(indexBefore) {
		t.Fatal("T016 review-state generation modified the real Git index")
	}
	if countAfter := gitOutput(t, repository, "count-objects", "-v"); countAfter != countBefore {
		t.Fatalf("T016 review-state generation modified the real object store:\nbefore=%s\nafter=%s", countBefore, countAfter)
	}

	pipe := filepath.Join(repository, "unsupported.pipe")
	if output, commandErr := exec.Command("mkfifo", pipe).CombinedOutput(); commandErr != nil {
		t.Fatalf("mkfifo: %v\n%s", commandErr, output)
	}
	if output, commandErr := snapshot(); commandErr == nil || !strings.Contains(output, "unsupported node") {
		t.Fatalf("special source node check = %v, output=%q", commandErr, output)
	}
}

func gitOutput(t *testing.T, repository string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func repositoryMakefile(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(filepath.Dir(workingDirectory), "Makefile")
}

func replaceMakeEnvironment(environment []string, assignments ...string) []string {
	replacements := make(map[string]string, len(assignments))
	for _, assignment := range assignments {
		name, _, _ := strings.Cut(assignment, "=")
		replacements[name] = assignment
	}
	result := make([]string, 0, len(environment)+len(assignments))
	for _, value := range environment {
		name, _, _ := strings.Cut(value, "=")
		if _, replaced := replacements[name]; !replaced {
			result = append(result, value)
		}
	}
	for _, assignment := range assignments {
		result = append(result, assignment)
	}
	return result
}
