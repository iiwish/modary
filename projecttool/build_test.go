package projecttool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iiwish/modary/appkit"
)

type cancelOnBuildTemporaryContext struct {
	context.Context
	cancel    context.CancelFunc
	directory string
	suffix    string
	once      sync.Once
}

func (ctx *cancelOnBuildTemporaryContext) Err() error {
	entries, err := os.ReadDir(ctx.directory)
	if err == nil {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".modary-") && strings.HasSuffix(entry.Name(), ctx.suffix) {
				ctx.once.Do(ctx.cancel)
				break
			}
		}
	}
	return ctx.Context.Err()
}

func TestBuildVerifiesChecksAndAtomicallyRunsOnlyGoBuild(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	createFixtureBuildPackage(t, project)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, filepath.FromSlash(project.Manifest().Build.Output))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	tooling := installFakeBuildTools(t, `
output=""
: > "$MODARY_GO_LOG"
[ "$GOFLAGS" = "" ] || exit 94
[ "$GOENV" = "off" ] || exit 95
[ "$GOWORK" = "off" ] || exit 96
[ "$GO111MODULE" = "on" ] || exit 93
[ "$GOTOOLCHAIN" = "local" ] || exit 92
[ -z "${GOROOT+x}" ] || exit 91
for argument in "$@"; do printf '%s\n' "$argument" >> "$MODARY_GO_LOG"; done
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then output="$2"; shift 2; else shift; fi
done
if [ -z "$output" ]; then exit 97; fi
printf 'new compiled binary\n' > "$output"
printf 'compiler stdout\n'
printf 'compiler stderr\n' >&2
`)
	t.Setenv("GOFLAGS", "-toolexec=node -race")
	t.Setenv("GOENV", filepath.Join(t.TempDir(), "untrusted-goenv"))
	t.Setenv("GOWORK", filepath.Join(t.TempDir(), "untrusted.go.work"))
	t.Setenv("GO111MODULE", "off")
	t.Setenv("GOTOOLCHAIN", "untrusted+auto")
	t.Setenv("GOROOT", filepath.Join(t.TempDir(), "untrusted-goroot"))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	counters := &inspectionCounters{}
	result, err := project.Build(context.Background(), fixtureDefinition(counters, true), BuildOptions{Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if result.Output != project.Manifest().Build.Output {
		t.Fatalf("Output = %q", result.Output)
	}
	if got := string(readFixtureFile(t, root, result.Output)); got != "new compiled binary\n" {
		t.Fatalf("binary = %q", got)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("binary mode = %o, want 755", info.Mode().Perm())
	}
	if stdout.String() != "compiler stdout\n" || stderr.String() != "compiler stderr\n" {
		t.Fatalf("compiler output = %q / %q", stdout.String(), stderr.String())
	}
	arguments := strings.Split(strings.TrimSpace(string(readFileForTest(t, tooling.goLog))), "\n")
	if len(arguments) != 7 || arguments[0] != "build" || arguments[1] != "-mod=readonly" ||
		arguments[2] != "-buildvcs=false" || arguments[3] != "-trimpath" || arguments[4] != "-o" ||
		arguments[6] != "./cmd/example-app" {
		t.Fatalf("Go arguments = %q", arguments)
	}
	if !filepath.IsAbs(arguments[5]) || strings.HasPrefix(arguments[5], project.Root()+string(filepath.Separator)) || filepath.Base(arguments[5]) != buildStagingOutputName {
		t.Fatalf("compiler output was not isolated from the project tree: %q", arguments[5])
	}
	if _, err := os.Stat(filepath.Dir(arguments[5])); !os.IsNotExist(err) {
		t.Fatalf("private compiler staging directory remains: %v", err)
	}
	if strings.Contains(string(readFileForTest(t, tooling.goLog)), "test") {
		t.Fatalf("Build implicitly ran tests: %s", readFileForTest(t, tooling.goLog))
	}
	if _, err := os.Stat(tooling.frontendMarker); !os.IsNotExist(err) {
		t.Fatalf("frontend tool was invoked: %v", err)
	}
	assertNoInspectionSideEffects(t, counters)
	assertNoTemporaryArtifacts(t, root)
}

func TestBuildRejectsDriftBeforeGoInvocation(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	tooling := installFakeBuildTools(t, `exit 91`)
	_, err := project.Build(context.Background(), fixtureDefinition(&inspectionCounters{}, false), BuildOptions{Stdout: io.Discard, Stderr: io.Discard})
	if !errors.Is(err, ErrDrift) {
		t.Fatalf("Build error = %v, want ErrDrift", err)
	}
	var drift *DriftError
	if !errors.As(err, &drift) || len(drift.Items) != 3 {
		t.Fatalf("Build drift = %#v", drift)
	}
	if _, err := os.Stat(tooling.goLog); !os.IsNotExist(err) {
		t.Fatalf("Go was invoked before drift failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "dist")); !os.IsNotExist(err) {
		t.Fatalf("Build created output directory before drift failed: %v", err)
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestBuildRejectsMissingPackageBeforeGoInvocation(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err != nil {
		t.Fatal(err)
	}
	tooling := installFakeBuildTools(t, `exit 91`)
	if _, err := project.Build(context.Background(), fixtureDefinition(&inspectionCounters{}, false), BuildOptions{Stdout: io.Discard, Stderr: io.Discard}); err == nil {
		t.Fatal("Build accepted a missing package directory")
	}
	if _, err := os.Stat(tooling.goLog); !os.IsNotExist(err) {
		t.Fatalf("Go was invoked for a missing package: %v", err)
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestBuildFailurePreservesPriorBinaryAndRemovesTemporaryOutput(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	createFixtureBuildPackage(t, project)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, filepath.FromSlash(project.Manifest().Build.Output))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("trusted old binary"), 0o711); err != nil {
		t.Fatal(err)
	}
	tooling := installFakeBuildTools(t, `
output=""
: > "$MODARY_GO_LOG"
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then output="$2"; shift 2; else shift; fi
done
printf 'partial untrusted output' > "$output"
exit 23
`)
	counters := &inspectionCounters{}
	if _, err := project.Build(context.Background(), fixtureDefinition(counters, false), BuildOptions{Stdout: io.Discard, Stderr: io.Discard}); err == nil {
		t.Fatal("Build succeeded")
	}
	if got := string(readFileForTest(t, target)); got != "trusted old binary" {
		t.Fatalf("prior binary changed: %q", got)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o711 {
		t.Fatalf("prior binary mode changed to %o", info.Mode().Perm())
	}
	if _, err := os.Stat(tooling.goLog); err != nil {
		t.Fatalf("fake Go was not invoked: %v", err)
	}
	if _, err := os.Stat(tooling.frontendMarker); !os.IsNotExist(err) {
		t.Fatalf("frontend tool was invoked: %v", err)
	}
	assertNoInspectionSideEffects(t, counters)
	assertNoTemporaryArtifacts(t, root)
}

func TestBuildRejectsRootPathnameSwapDuringGoCommand(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ProjectManifestName), []byte(validProjectManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	project := loadFixtureProject(t, root)
	createFixtureBuildPackage(t, project)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, filepath.FromSlash(project.Manifest().Build.Output))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("trusted old binary"), 0o711); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(parent, "opened-root")
	installFakeBuildTools(t, `
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then output="$2"; shift 2; else shift; fi
done
mv "$MODARY_PROJECT_ROOT" "$MODARY_MOVED_ROOT"
mkdir -p "$MODARY_PROJECT_ROOT/dist" "$MODARY_PROJECT_ROOT/cmd/example-app"
cp "$MODARY_MOVED_ROOT/modary.yaml" "$MODARY_PROJECT_ROOT/modary.yaml"
printf 'untrusted replacement-root binary' > "$output"
`)
	t.Setenv("MODARY_PROJECT_ROOT", root)
	t.Setenv("MODARY_MOVED_ROOT", moved)

	_, err := project.Build(context.Background(), fixtureDefinition(&inspectionCounters{}, false), BuildOptions{Stdout: io.Discard, Stderr: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "root pathname changed") {
		t.Fatalf("Build error = %v", err)
	}
	if got := string(readFileForTest(t, filepath.Join(moved, filepath.FromSlash(project.Manifest().Build.Output)))); got != "trusted old binary" {
		t.Fatalf("trusted binary changed: %q", got)
	}
	if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(project.Manifest().Build.Output))); !os.IsNotExist(statErr) {
		t.Fatalf("replacement root retained build output: %v", statErr)
	}
	assertNoTemporaryArtifacts(t, moved)
	assertNoTemporaryArtifacts(t, root)
}

func TestBuildOutputCannotFollowSwappedProjectParent(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	createFixtureBuildPackage(t, project)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err != nil {
		t.Fatal(err)
	}
	outputDirectory := filepath.Join(root, "dist")
	if err := os.Mkdir(outputDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	outsideTarget := filepath.Join(outside, "example-app")
	if err := os.WriteFile(outsideTarget, []byte("outside sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	tooling := installFakeBuildTools(t, `
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then output="$2"; shift 2; else shift; fi
done
rmdir "$MODARY_OUTPUT_PARENT"
ln -s "$MODARY_OUTSIDE" "$MODARY_OUTPUT_PARENT"
printf 'staged compiler output' > "$output"
printf '%s' "$output" > "$MODARY_GO_LOG"
`)
	t.Setenv("MODARY_OUTPUT_PARENT", outputDirectory)
	t.Setenv("MODARY_OUTSIDE", outside)

	if _, err := project.Build(context.Background(), fixtureDefinition(&inspectionCounters{}, false), BuildOptions{Stdout: io.Discard, Stderr: io.Discard}); err == nil {
		t.Fatal("Build accepted a swapped output parent")
	}
	if got := string(readFileForTest(t, outsideTarget)); got != "outside sentinel" {
		t.Fatalf("outside target was modified: %q", got)
	}
	stagedPath := string(readFileForTest(t, tooling.goLog))
	if _, err := os.Stat(filepath.Dir(stagedPath)); !os.IsNotExist(err) {
		t.Fatalf("private compiler staging directory remains: %v", err)
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestBuildHonorsCancellationAndContainsWriterFailures(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	createFixtureBuildPackage(t, project)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err != nil {
		t.Fatal(err)
	}
	tooling := installFakeBuildTools(t, `
output=""
: > "$MODARY_GO_LOG"
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then output="$2"; shift 2; else shift; fi
done
printf 'binary' > "$output"
printf 'output that reaches writer\n'
`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := project.Build(ctx, fixtureDefinition(&inspectionCounters{}, false), BuildOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Build error = %v", err)
	}
	if _, err := os.Stat(tooling.goLog); !os.IsNotExist(err) {
		t.Fatalf("Go ran for canceled context: %v", err)
	}

	panicOutput := panicWriter{}
	_, err := project.Build(context.Background(), fixtureDefinition(&inspectionCounters{}, false), BuildOptions{Stdout: panicOutput, Stderr: io.Discard})
	if !errors.Is(err, ErrCallbackPanic) {
		t.Fatalf("panic writer error = %v, want ErrCallbackPanic", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(project.Manifest().Build.Output))); !os.IsNotExist(statErr) {
		t.Fatalf("writer failure installed binary: %v", statErr)
	}
	assertNoTemporaryArtifacts(t, root)

	var typedNilCause *nilWriteError
	_, err = project.Build(context.Background(), fixtureDefinition(&inspectionCounters{}, false), BuildOptions{
		Stdout: typedNilErrorWriter{},
		Stderr: io.Discard,
	})
	assertTypedNilWriterFailure(t, err, typedNilCause)
	if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(project.Manifest().Build.Output))); !os.IsNotExist(statErr) {
		t.Fatalf("typed-nil writer failure installed binary: %v", statErr)
	}
	assertNoTemporaryArtifacts(t, root)

	var typedNil *bytes.Buffer
	if _, err := project.Build(context.Background(), fixtureDefinition(&inspectionCounters{}, false), BuildOptions{Stdout: typedNil}); !errors.Is(err, ErrUsage) {
		t.Fatalf("typed nil writer error = %v, want ErrUsage", err)
	}
}

func TestBuildCancellationDuringGoCommandPreservesBaseline(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	createFixtureBuildPackage(t, project)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, filepath.FromSlash(project.Manifest().Build.Output))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("trusted baseline"), 0o711); err != nil {
		t.Fatal(err)
	}
	installFakeBuildTools(t, `
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then output="$2"; shift 2; else shift; fi
done
	printf 'partial output' > "$output"
	(sleep 30) &
	printf '%s' "$!" > "$MODARY_CHILD_PID_LOG"
	printf 'started' > "$MODARY_GO_LOG"
	while :; do :; done
`)
	childPIDLog := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("MODARY_CHILD_PID_LOG", childPIDLog)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := project.Build(ctx, fixtureDefinition(&inspectionCounters{}, false), BuildOptions{Stdout: io.Discard, Stderr: io.Discard})
		done <- err
	}()
	childPID, waitErr := waitForProcessIDForTest(childPIDLog, 5*time.Second)
	if waitErr != nil {
		cancel()
		buildErr := <-done
		t.Fatalf("fake Go command did not publish its child process id: %v; Build cleanup: %v", waitErr, buildErr)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Build error = %v, want context.Canceled", err)
	}
	if got := string(readFileForTest(t, target)); got != "trusted baseline" {
		t.Fatalf("baseline changed: %q", got)
	}
	deadline := time.Now().Add(2 * time.Second)
	for processExistsForTest(childPID) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if processExistsForTest(childPID) {
		t.Fatalf("compiler descendant %d survived context cancellation", childPID)
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestBuildCancellationDuringBackupPreservesBaseline(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	createFixtureBuildPackage(t, project)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, filepath.FromSlash(project.Manifest().Build.Output))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	baseline := bytes.Repeat([]byte("trusted baseline\n"), 16*1024)
	if err := os.WriteFile(target, baseline, 0o711); err != nil {
		t.Fatal(err)
	}
	installFakeBuildTools(t, `
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then output="$2"; shift 2; else shift; fi
done
printf 'new compiled binary\n' > "$output"
`)

	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cancelOnBuildTemporaryContext{
		Context:   base,
		cancel:    cancel,
		directory: filepath.Dir(target),
		suffix:    ".bak",
	}
	_, err := project.Build(ctx, fixtureDefinition(&inspectionCounters{}, false), BuildOptions{Stdout: io.Discard, Stderr: io.Discard})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Build error = %v, want context.Canceled", err)
	}
	if got := readFileForTest(t, target); !bytes.Equal(got, baseline) {
		t.Fatalf("baseline changed during canceled backup")
	}
	info, statErr := os.Stat(target)
	if statErr != nil || info.Mode().Perm() != 0o711 {
		t.Fatalf("baseline mode changed: info=%v err=%v", info, statErr)
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestBuildCancellationDuringStagedCopyLeavesNoOutput(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	createFixtureBuildPackage(t, project)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err != nil {
		t.Fatal(err)
	}
	installFakeBuildTools(t, `
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then output="$2"; shift 2; else shift; fi
done
printf 'new compiled binary\n' > "$output"
`)
	target := filepath.Join(root, filepath.FromSlash(project.Manifest().Build.Output))
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cancelOnBuildTemporaryContext{
		Context:   base,
		cancel:    cancel,
		directory: filepath.Dir(target),
		suffix:    ".build",
	}
	_, err := project.Build(ctx, fixtureDefinition(&inspectionCounters{}, false), BuildOptions{Stdout: io.Discard, Stderr: io.Discard})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Build error = %v, want context.Canceled", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("canceled staged copy installed output: %v", statErr)
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestBuildRejectsProjectOwnedCompilerStaging(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	createFixtureBuildPackage(t, project)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err != nil {
		t.Fatal(err)
	}
	tooling := installFakeBuildTools(t, `exit 90`)
	t.Setenv("TMPDIR", root)
	_, err := project.Build(context.Background(), fixtureDefinition(&inspectionCounters{}, false), BuildOptions{Stdout: io.Discard, Stderr: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "must be outside the project root") {
		t.Fatalf("Build error = %v", err)
	}
	if _, statErr := os.Stat(tooling.goLog); !os.IsNotExist(statErr) {
		t.Fatalf("Go ran with project-owned staging: %v", statErr)
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "modary-build-") {
			t.Fatalf("rejected project-owned staging remains: %s", entry.Name())
		}
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestBuildRejectsEUIDOwnedWorldWritableNonStickyCompilerStaging(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	createFixtureBuildPackage(t, project)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err != nil {
		t.Fatal(err)
	}
	temporaryRoot := t.TempDir()
	if err := os.Chmod(temporaryRoot, 0o777); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(temporaryRoot, 0o700) })
	tooling := installFakeBuildTools(t, `exit 90`)
	t.Setenv("TMPDIR", temporaryRoot)

	_, err := project.Build(context.Background(), fixtureDefinition(&inspectionCounters{}, false), BuildOptions{Stdout: io.Discard, Stderr: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "must be root-owned and sticky") {
		t.Fatalf("Build error = %v", err)
	}
	if _, statErr := os.Stat(tooling.goLog); !os.IsNotExist(statErr) {
		t.Fatalf("Go ran with an insecure temporary parent: %v", statErr)
	}
	entries, readErr := os.ReadDir(temporaryRoot)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("rejected compiler staging remains: entries=%v err=%v", entries, readErr)
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestBuildRejectsEUIDOwnedWorldWritableNonStickyStagingAncestor(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	createFixtureBuildPackage(t, project)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	insecureAncestor := filepath.Join(base, "insecure")
	temporaryRoot := filepath.Join(insecureAncestor, "tmp")
	if err := os.Mkdir(insecureAncestor, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(temporaryRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(insecureAncestor, 0o777); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(insecureAncestor, 0o700) })
	tooling := installFakeBuildTools(t, `exit 90`)
	t.Setenv("TMPDIR", temporaryRoot)

	_, err := project.Build(context.Background(), fixtureDefinition(&inspectionCounters{}, false), BuildOptions{Stdout: io.Discard, Stderr: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "must be root-owned and sticky") {
		t.Fatalf("Build error = %v", err)
	}
	if _, statErr := os.Stat(tooling.goLog); !os.IsNotExist(statErr) {
		t.Fatalf("Go ran with an insecure temporary ancestor: %v", statErr)
	}
	entries, readErr := os.ReadDir(temporaryRoot)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("rejected compiler staging remains: entries=%v err=%v", entries, readErr)
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestBuildRejectsSymlinkedCompilerStagingInsideProject(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	createFixtureBuildPackage(t, project)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err != nil {
		t.Fatal(err)
	}
	stagingParent := filepath.Join(root, "compiler-staging")
	if err := os.Mkdir(stagingParent, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "project-staging-alias")
	if err := os.Symlink(stagingParent, alias); err != nil {
		t.Fatal(err)
	}
	tooling := installFakeBuildTools(t, `exit 90`)
	t.Setenv("TMPDIR", alias)
	_, err := project.Build(context.Background(), fixtureDefinition(&inspectionCounters{}, false), BuildOptions{Stdout: io.Discard, Stderr: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "must be outside the project root") {
		t.Fatalf("Build error = %v", err)
	}
	if _, statErr := os.Stat(tooling.goLog); !os.IsNotExist(statErr) {
		t.Fatalf("Go ran with symlinked project staging: %v", statErr)
	}
	entries, readErr := os.ReadDir(stagingParent)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("rejected symlinked staging remains: entries=%v err=%v", entries, readErr)
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestBuildPassesOnlyCanonicalValidatedTemporaryDirectoryToGo(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	createFixtureBuildPackage(t, project)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err != nil {
		t.Fatal(err)
	}

	temporaryRoot := filepath.Join(t.TempDir(), "validated-temp")
	if err := os.Mkdir(temporaryRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(temporaryRoot)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "temporary-alias")
	if err := os.Symlink(temporaryRoot, alias); err != nil {
		t.Fatal(err)
	}

	installFakeBuildTools(t, `
[ "$TMPDIR" = "$MODARY_EXPECTED_TMPDIR" ] || exit 81
[ "$GOTMPDIR" = "$MODARY_EXPECTED_TMPDIR" ] || exit 82
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then output="$2"; shift 2; else shift; fi
done
printf 'canonical temporary environment\n' > "$output"
`)
	t.Setenv("TMPDIR", alias)
	t.Setenv("GOTMPDIR", filepath.Join(root, "untrusted-go-temporary"))
	t.Setenv("MODARY_EXPECTED_TMPDIR", canonical)

	result, err := project.Build(
		context.Background(),
		fixtureDefinition(&inspectionCounters{}, false),
		BuildOptions{Stdout: io.Discard, Stderr: io.Discard},
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := string(readFixtureFile(t, root, result.Output)); got != "canonical temporary environment\n" {
		t.Fatalf("binary = %q", got)
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestBuildStagingRejectsMismatchedRetainedRoot(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	original := t.TempDir()
	other := t.TempDir()
	var err error
	original, err = filepath.EvalSymlinks(original)
	if err != nil {
		t.Fatal(err)
	}
	other, err = filepath.EvalSymlinks(other)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(original, 0o700); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(original)
	if err != nil {
		t.Fatal(err)
	}
	parentDirectory := filepath.Dir(original)
	parent, err := os.OpenRoot(parentDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	parentInfo, err := parent.Stat(".")
	if err != nil {
		t.Fatal(err)
	}
	parentSecurity, err := parent.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	defer parentSecurity.Close()
	ancestors, err := openBuildStagingAncestors(parentDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer closeBuildStagingAncestors(ancestors)
	security, err := os.Open(original)
	if err != nil {
		t.Fatal(err)
	}
	defer security.Close()
	root, err := os.OpenRoot(other)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	staging := &buildStaging{
		parentDirectory: parentDirectory,
		parentInfo:      parentInfo,
		parent:          parent,
		parentSecurity:  parentSecurity,
		ancestors:       ancestors,
		name:            filepath.Base(original),
		directory:       original,
		info:            info,
		security:        security,
		root:            root,
	}
	if err := staging.validatePathBinding(); err == nil || !strings.Contains(err.Error(), "changed identity") {
		t.Fatalf("mismatched retained Root error = %v", err)
	}
}

func TestBuildRedactsHostileWriterErrors(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	createFixtureBuildPackage(t, project)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err != nil {
		t.Fatal(err)
	}
	installFakeBuildTools(t, `
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then output="$2"; shift 2; else shift; fi
done
printf 'compiled binary\n' > "$output"
printf 'compiler output\n'
`)
	failure := &hostileBuildWriterError{entered: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, err := project.Build(context.Background(), fixtureDefinition(&inspectionCounters{}, false), BuildOptions{
			Stdout: hostileBuildWriter{failure: failure},
			Stderr: io.Discard,
		})
		done <- err
	}()
	var err error
	select {
	case err = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Build blocked while formatting a writer error")
	}
	if err == nil || !strings.Contains(err.Error(), "build stdout failed") || strings.Contains(err.Error(), "writer-secret") {
		t.Fatalf("Build error = %v", err)
	}
	if !errors.Is(err, failure) {
		t.Fatalf("Build error does not retain the writer cause: %v", err)
	}
	select {
	case <-failure.entered:
		t.Fatal("hostile writer Error method was invoked")
	default:
	}
	if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(project.Manifest().Build.Output))); !os.IsNotExist(statErr) {
		t.Fatalf("writer failure installed output: %v", statErr)
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestBuildReapsInheritedCompilerProcessGroup(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	createFixtureBuildPackage(t, project)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err != nil {
		t.Fatal(err)
	}
	installFakeBuildTools(t, `
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then output="$2"; shift 2; else shift; fi
done
printf 'compiled binary\n' > "$output"
(sleep 30; printf 'late inherited output\n') &
printf '%s' "$!" > "$MODARY_CHILD_PID_LOG"
`)
	childPIDLog := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("MODARY_CHILD_PID_LOG", childPIDLog)

	started := time.Now()
	result, err := project.Build(context.Background(), fixtureDefinition(&inspectionCounters{}, false), BuildOptions{Stdout: io.Discard, Stderr: io.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if elapsed := time.Since(started); elapsed > buildCommandWaitDelay+3*time.Second {
		t.Fatalf("Build waited too long for inherited compiler pipe: %s", elapsed)
	}
	if result.Output != project.Manifest().Build.Output {
		t.Fatalf("Build output = %q", result.Output)
	}
	if got := string(readFixtureFile(t, root, result.Output)); got != "compiled binary\n" {
		t.Fatalf("installed binary = %q", got)
	}
	childPID, parseErr := readProcessIDForTest(childPIDLog)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	waitForProcessExitForTest(t, childPID)
	assertNoTemporaryArtifacts(t, root)
}

func TestBuildFailureReapsInheritedCompilerProcessGroup(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	createFixtureBuildPackage(t, project)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err != nil {
		t.Fatal(err)
	}
	installFakeBuildTools(t, `
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then output="$2"; shift 2; else shift; fi
done
printf 'partial compiler output\n' > "$output"
(sleep 30; printf 'late inherited output\n') &
printf '%s' "$!" > "$MODARY_CHILD_PID_LOG"
exit 23
`)
	childPIDLog := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("MODARY_CHILD_PID_LOG", childPIDLog)

	started := time.Now()
	_, err := project.Build(context.Background(), fixtureDefinition(&inspectionCounters{}, false), BuildOptions{Stdout: io.Discard, Stderr: io.Discard})
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Build error = %v, want Go process exit error", err)
	}
	if elapsed := time.Since(started); elapsed > buildCommandWaitDelay+3*time.Second {
		t.Fatalf("failed Build waited too long for inherited compiler pipe: %s", elapsed)
	}
	if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(project.Manifest().Build.Output))); !os.IsNotExist(statErr) {
		t.Fatalf("failed compiler installed binary: %v", statErr)
	}
	childPID, parseErr := readProcessIDForTest(childPIDLog)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	waitForProcessExitForTest(t, childPID)
	assertNoTemporaryArtifacts(t, root)
}

func TestConcurrentBuildsSerializeAndKeepLiveTargetReadable(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	createFixtureBuildPackage(t, project)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, filepath.FromSlash(project.Manifest().Build.Output))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("trusted baseline"), 0o711); err != nil {
		t.Fatal(err)
	}
	installFakeBuildTools(t, `
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then output="$2"; shift 2; else shift; fi
done
if ! mkdir "$MODARY_BUILD_LOCK" 2>/dev/null; then exit 88; fi
trap 'rmdir "$MODARY_BUILD_LOCK"' EXIT
sleep 0.02
printf 'compiled binary\n' > "$output"
`)
	t.Setenv("MODARY_BUILD_LOCK", filepath.Join(t.TempDir(), "build.lock"))

	stopReading := make(chan struct{})
	readerDone := make(chan error, 1)
	go func() {
		for {
			select {
			case <-stopReading:
				readerDone <- nil
				return
			default:
				data, err := os.ReadFile(target)
				if err != nil {
					readerDone <- err
					return
				}
				if len(data) == 0 {
					readerDone <- fmt.Errorf("observed empty live binary")
					return
				}
			}
		}
	}()

	const workers = 8
	start := make(chan struct{})
	results := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := project.Build(context.Background(), fixtureDefinition(&inspectionCounters{}, false), BuildOptions{Stdout: io.Discard, Stderr: io.Discard})
			results <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(stopReading)
	if err := <-readerDone; err != nil {
		t.Fatalf("live build output became unreadable: %v", err)
	}
	for err := range results {
		if err != nil {
			t.Fatalf("concurrent Build: %v", err)
		}
	}
	if got := string(readFileForTest(t, target)); got != "compiled binary\n" {
		t.Fatalf("final binary = %q", got)
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestBuildRejectsNilContextAndNilProject(t *testing.T) {
	var project *Project
	if _, err := project.Build(nil, appkit.Definition{}, BuildOptions{}); !errors.Is(err, ErrContextRequired) {
		t.Fatalf("nil context error = %v", err)
	}
	if _, err := project.Build(context.Background(), appkit.Definition{}, BuildOptions{}); err == nil {
		t.Fatal("nil Project Build succeeded")
	}
}

func TestGoBuildEnvironmentDisablesAmbientGoConfigurationInjection(t *testing.T) {
	got := goBuildEnvironment([]string{
		"PATH=/tooling",
		"GOFLAGS=-toolexec=node -race",
		"goenv=/untrusted/goenv",
		"GOWORK=/consumer/go.work",
		"GO111MODULE=off",
		"GOTOOLCHAIN=untrusted+auto",
		"GOROOT=/untrusted/go-root",
		"TMPDIR=/untrusted/tmp-alias",
		"gotmpdir=/untrusted/go-tmp",
	}, "/validated/tmp")
	want := []string{
		"PATH=/tooling",
		"GOFLAGS=",
		"GOENV=off",
		"GOWORK=off",
		"GO111MODULE=on",
		"GOTOOLCHAIN=local",
		"TMPDIR=/validated/tmp",
		"GOTMPDIR=/validated/tmp",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("environment = %v, want %v", got, want)
	}
}

type fakeBuildTools struct {
	goLog          string
	frontendMarker string
}

func installFakeBuildTools(t *testing.T, goBody string) fakeBuildTools {
	t.Helper()
	directory := t.TempDir()
	goLog := filepath.Join(directory, "go.log")
	frontendMarker := filepath.Join(directory, "frontend-called")
	writeFakeExecutable(t, filepath.Join(directory, "go"), goBody)
	for _, name := range []string{"node", "npm", "pnpm"} {
		writeFakeExecutable(t, filepath.Join(directory, name), fmt.Sprintf("printf '%s' > %q\nexit 99", name, frontendMarker))
	}
	t.Setenv("MODARY_GO_LOG", goLog)
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	return fakeBuildTools{goLog: goLog, frontendMarker: frontendMarker}
}

func writeFakeExecutable(t *testing.T, name, body string) {
	t.Helper()
	data := "#!/bin/sh\nset -eu\n" + strings.TrimSpace(body) + "\n"
	if err := os.WriteFile(name, []byte(data), 0o755); err != nil {
		t.Fatal(err)
	}
}

func createFixtureBuildPackage(t *testing.T, project *Project) {
	t.Helper()
	packagePath := strings.TrimPrefix(project.Manifest().Build.Package, "./")
	if err := os.MkdirAll(filepath.Join(project.Root(), filepath.FromSlash(packagePath)), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readFileForTest(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func waitForProcessIDForTest(name string, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		pid, err := readProcessIDForTest(name)
		if err == nil {
			return pid, nil
		}
		lastErr = err
		if !time.Now().Before(deadline) {
			return 0, fmt.Errorf("wait for child process id: %w", lastErr)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitForProcessExitForTest(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for processExistsForTest(pid) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if processExistsForTest(pid) {
		t.Fatalf("compiler descendant %d survived Build cleanup", pid)
	}
}

type panicWriter struct{}

func (panicWriter) Write([]byte) (int, error) { panic("writer panic must be contained") }

type hostileBuildWriter struct{ failure error }

func (writer hostileBuildWriter) Write(data []byte) (int, error) { return len(data), writer.failure }

type hostileBuildWriterError struct{ entered chan struct{} }

func (err *hostileBuildWriterError) Error() string {
	close(err.entered)
	select {}
}

func (*hostileBuildWriterError) Is(error) bool { panic("hostile writer Is invoked") }
func (*hostileBuildWriterError) Unwrap() error { panic("hostile writer Unwrap invoked") }
