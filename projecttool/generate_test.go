package projecttool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGenerateIsDeterministicPerFileAtomicAndIdempotent(t *testing.T) {
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	firstCounters := &inspectionCounters{}
	first, err := project.Generate(fixtureDefinition(firstCounters, false))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	wantPaths := []string{
		"internal/generated/action_catalog.json",
		"internal/generated/module_graph.json",
		"web/src/generated/actionContracts.ts",
	}
	if !reflect.DeepEqual(first.Written, wantPaths) || len(first.Unchanged) != 0 {
		t.Fatalf("first Generation = %#v, want Written %v", first, wantPaths)
	}
	if first.Unchanged == nil {
		t.Fatal("first Generation returned a nil unchanged list")
	}
	assertNoInspectionSideEffects(t, firstCounters)

	before := make(map[string][]byte, len(wantPaths))
	modTimes := make(map[string]time.Time, len(wantPaths))
	for _, name := range wantPaths {
		before[name] = append([]byte(nil), readFixtureFile(t, root, name)...)
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		modTimes[name] = info.ModTime()
		if name != project.Manifest().Outputs.TypeScript {
			var document map[string]any
			if err := json.Unmarshal(before[name], &document); err != nil {
				t.Fatalf("generated %s is not JSON: %v", name, err)
			}
			if document["schema_version"] == "" {
				t.Fatalf("generated %s has no schema version", name)
			}
		}
	}
	if typescript := string(before[project.Manifest().Outputs.TypeScript]); !strings.Contains(typescript, `"records.archive"`) || !strings.Contains(typescript, `"records.create"`) {
		t.Fatalf("TypeScript catalog is incomplete:\n%s", typescript)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(project.Manifest().Build.Output))); !os.IsNotExist(err) {
		t.Fatalf("Generate created build output: %v", err)
	}

	time.Sleep(5 * time.Millisecond)
	secondCounters := &inspectionCounters{}
	second, err := project.Generate(fixtureDefinition(secondCounters, true))
	if err != nil {
		t.Fatalf("Generate shuffled Definition: %v", err)
	}
	if len(second.Written) != 0 || !reflect.DeepEqual(second.Unchanged, wantPaths) {
		t.Fatalf("second Generation = %#v, want Unchanged %v", second, wantPaths)
	}
	if second.Written == nil {
		t.Fatal("second Generation returned a nil written list")
	}
	assertNoInspectionSideEffects(t, secondCounters)
	for _, name := range wantPaths {
		if after := readFixtureFile(t, root, name); !bytes.Equal(after, before[name]) {
			t.Fatalf("%s changed across identical generation", name)
		}
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(modTimes[name]) {
			t.Fatalf("%s was replaced despite byte equality", name)
		}
	}
	drift, err := project.Check(fixtureDefinition(&inspectionCounters{}, true))
	if err != nil || len(drift) != 0 {
		t.Fatalf("Check after generation = %#v, %v", drift, err)
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestCheckReportsAllSortedDriftWithoutWrites(t *testing.T) {
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	definition := fixtureDefinition(&inspectionCounters{}, false)

	before := snapshotTree(t, root)
	drift, err := project.Check(definition)
	if err != nil {
		t.Fatalf("Check missing artifacts: %v", err)
	}
	wantMissing := []Drift{
		{Path: "internal/generated/action_catalog.json", Status: DriftMissing},
		{Path: "internal/generated/module_graph.json", Status: DriftMissing},
		{Path: "web/src/generated/actionContracts.ts", Status: DriftMissing},
	}
	if !reflect.DeepEqual(drift, wantMissing) {
		t.Fatalf("missing drift = %#v, want %#v", drift, wantMissing)
	}
	driftErr := &DriftError{Items: drift}
	if got, want := driftErr.Error(), "generated artifact drift: internal/generated/action_catalog.json (missing), internal/generated/module_graph.json (missing), web/src/generated/actionContracts.ts (missing)"; got != want {
		t.Fatalf("DriftError = %q, want %q", got, want)
	}
	if !errors.Is(driftErr, ErrDrift) {
		t.Fatal("DriftError does not unwrap to ErrDrift")
	}
	if after := snapshotTree(t, root); !reflect.DeepEqual(after, before) {
		t.Fatalf("Check mutated project\nbefore: %#v\nafter:  %#v", before, after)
	}

	if _, err := project.Generate(definition); err != nil {
		t.Fatal(err)
	}
	actions := filepath.Join(root, filepath.FromSlash(project.Manifest().Outputs.Actions))
	if err := os.WriteFile(actions, []byte("different"), 0o644); err != nil {
		t.Fatal(err)
	}
	graph := filepath.Join(root, filepath.FromSlash(project.Manifest().Outputs.Graph))
	if err := os.Remove(graph); err != nil {
		t.Fatal(err)
	}
	before = snapshotTree(t, root)
	drift, err = project.Check(definition)
	if err != nil {
		t.Fatal(err)
	}
	want := []Drift{
		{Path: project.Manifest().Outputs.Actions, Status: DriftDifferent},
		{Path: project.Manifest().Outputs.Graph, Status: DriftMissing},
	}
	if !reflect.DeepEqual(drift, want) {
		t.Fatalf("drift = %#v, want %#v", drift, want)
	}
	if after := snapshotTree(t, root); !reflect.DeepEqual(after, before) {
		t.Fatalf("drift Check mutated project\nbefore: %#v\nafter:  %#v", before, after)
	}
}

func TestGenerateValidationFailurePreservesEveryArtifact(t *testing.T) {
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	valid := fixtureDefinition(&inspectionCounters{}, false)
	if _, err := project.Generate(valid); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, root)
	invalid := fixtureDefinition(&inspectionCounters{}, false)
	invalid.Modules[1].Definition.Actions[0].Descriptor.OutputSchema = []byte(`{"type":false}`)
	if _, err := project.Generate(invalid); err == nil {
		t.Fatal("Generate accepted an invalid Definition")
	}
	if after := snapshotTree(t, root); !reflect.DeepEqual(after, before) {
		t.Fatalf("validation failure changed project\nbefore: %#v\nafter:  %#v", before, after)
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestGeneratedArtifactSizeLimitFailsClosed(t *testing.T) {
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	graph := filepath.Join(root, filepath.FromSlash(project.Manifest().Outputs.Graph))
	if err := os.MkdirAll(filepath.Dir(graph), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(graph, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(MaximumGeneratedArtifactBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(graph)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err == nil {
		t.Fatal("Generate accepted an oversized existing artifact")
	}
	after, err := os.Stat(graph)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("oversized artifact changed: before=%v after=%v", before, after)
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestGeneratePreparationFailureCleansEarlierTemporaryFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on Windows")
	}
	manifest := strings.Replace(validProjectManifest, "internal/generated/action_catalog.json", "a/actions.json", 1)
	manifest = strings.Replace(manifest, "internal/generated/module_graph.json", "z/graph.json", 1)
	manifest = strings.Replace(manifest, "  typescript: web/src/generated/actionContracts.ts\n", "", 1)
	root := writeFixtureProject(t, manifest)
	if err := os.MkdirAll(filepath.Join(root, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(root, "z")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	probe, probeErr := os.OpenFile(filepath.Join(locked, "probe"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if probeErr == nil {
		_ = probe.Close()
		_ = os.Remove(filepath.Join(locked, "probe"))
		t.Skip("current user can write to read-only directories")
	}

	project := loadFixtureProject(t, root)
	if _, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false)); err == nil {
		t.Fatal("Generate succeeded despite an unwritable destination")
	}
	if _, err := os.Stat(filepath.Join(root, "a", "actions.json")); !os.IsNotExist(err) {
		t.Fatalf("earlier output was installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "z", "graph.json")); !os.IsNotExist(err) {
		t.Fatalf("failed output was installed: %v", err)
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestConcurrentGenerateSerializesSameRootCalls(t *testing.T) {
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	const workers = 24
	start := make(chan struct{})
	errorsByWorker := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(reverse bool) {
			defer wait.Done()
			<-start
			counters := &inspectionCounters{}
			_, err := project.Generate(fixtureDefinition(counters, reverse))
			if err == nil && (counters.starts.Load() != 0 || counters.handlers.Load() != 0 || counters.opens.Load() != 0) {
				err = fmt.Errorf("inspection callbacks ran")
			}
			errorsByWorker <- err
		}(index%2 == 0)
	}
	close(start)
	wait.Wait()
	close(errorsByWorker)
	for err := range errorsByWorker {
		if err != nil {
			t.Fatalf("concurrent Generate: %v", err)
		}
	}
	for _, name := range []string{project.Manifest().Outputs.Actions, project.Manifest().Outputs.Graph} {
		if !json.Valid(readFixtureFile(t, root, name)) {
			t.Fatalf("%s is not valid JSON", name)
		}
	}
	drift, err := project.Check(fixtureDefinition(&inspectionCounters{}, false))
	if err != nil || len(drift) != 0 {
		t.Fatalf("Check after concurrent generation = %#v, %v", drift, err)
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestGenerateKeepsExistingArtifactsReadableDuringReplacement(t *testing.T) {
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	baseline := fixtureDefinition(&inspectionCounters{}, false)
	changed := fixtureDefinition(&inspectionCounters{}, false)
	changed.Modules[1].Definition.Actions[0].Descriptor.Title = "Updated fixture title"
	if _, err := project.Generate(baseline); err != nil {
		t.Fatal(err)
	}

	paths := []string{project.Manifest().Outputs.Actions, project.Manifest().Outputs.Graph}
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		for {
			select {
			case <-stop:
				done <- nil
				return
			default:
				for _, name := range paths {
					data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
					if err != nil {
						done <- err
						return
					}
					if !json.Valid(data) {
						done <- fmt.Errorf("observed incomplete generated artifact %s", name)
						return
					}
				}
			}
		}
	}()
	for index := range 50 {
		definition := baseline
		if index%2 != 0 {
			definition = changed
		}
		if _, err := project.Generate(definition); err != nil {
			close(stop)
			<-done
			t.Fatalf("Generate replacement %d: %v", index, err)
		}
	}
	close(stop)
	if err := <-done; err != nil {
		t.Fatalf("generated artifact became unreadable: %v", err)
	}
	assertNoTemporaryArtifacts(t, root)
}

func TestProjectRootGateSeparatesRootsAndHonorsCancellation(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	firstIdentity, err := os.Stat(firstRoot)
	if err != nil {
		t.Fatal(err)
	}
	secondIdentity, err := os.Stat(secondRoot)
	if err != nil {
		t.Fatal(err)
	}
	releaseFirst, err := acquireProjectRoot(context.Background(), firstRoot, firstIdentity)
	if err != nil {
		t.Fatal(err)
	}
	releaseSecond, err := acquireProjectRoot(context.Background(), secondRoot, secondIdentity)
	if err != nil {
		releaseFirst()
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	type acquisition struct {
		release func()
		err     error
	}
	acquired := make(chan acquisition, 1)
	go func() {
		release, acquireErr := acquireProjectRoot(ctx, firstRoot+"-alias", firstIdentity)
		acquired <- acquisition{release: release, err: acquireErr}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		select {
		case unexpected := <-acquired:
			if unexpected.release != nil {
				unexpected.release()
			}
			releaseSecond()
			releaseFirst()
			t.Fatalf("same-identity alias acquired a second gate: %v", unexpected.err)
		default:
		}
		projectRootGates.Lock()
		waitingOnFirst := false
		for _, gate := range projectRootGates.entries {
			if os.SameFile(gate.identity, firstIdentity) && gate.references == 2 {
				waitingOnFirst = true
				break
			}
		}
		projectRootGates.Unlock()
		if waitingOnFirst {
			break
		}
		if time.Now().After(deadline) {
			releaseSecond()
			releaseFirst()
			t.Fatal("same-identity alias did not join the existing gate")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	blocked := <-acquired
	if blocked.release != nil {
		blocked.release()
	}
	if !errors.Is(blocked.err, context.Canceled) {
		releaseSecond()
		releaseFirst()
		t.Fatalf("same-root acquire error = %v", blocked.err)
	}
	releaseSecond()
	releaseFirst()

	projectRootGates.Lock()
	remaining := len(projectRootGates.entries)
	projectRootGates.Unlock()
	if remaining != 0 {
		t.Fatalf("root gate registry retained %d entries", remaining)
	}
}

func TestOptionalTypeScriptOutputIsTrulyOptional(t *testing.T) {
	manifest := strings.Replace(validProjectManifest, "  typescript: web/src/generated/actionContracts.ts\n", "", 1)
	root := writeFixtureProject(t, manifest)
	project := loadFixtureProject(t, root)
	result, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{project.Manifest().Outputs.Actions, project.Manifest().Outputs.Graph}
	sort.Strings(want)
	if !reflect.DeepEqual(result.Written, want) {
		t.Fatalf("Written = %v, want %v", result.Written, want)
	}
	if _, err := os.Stat(filepath.Join(root, "web")); !os.IsNotExist(err) {
		t.Fatalf("optional TypeScript directory exists: %v", err)
	}
}

func TestRollbackReplacementsRestoresWholePriorBatch(t *testing.T) {
	rootDirectory := t.TempDir()
	root, err := os.OpenRoot(rootDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.Mkdir("generated", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootDirectory, "generated", "existing.json"), []byte("old existing"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := root.Rename("generated/existing.json", "generated/existing.backup"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootDirectory, "generated", "existing.json"), []byte("new existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootDirectory, "generated", "new.json"), []byte("new file"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes := []replacement{
		{
			artifact:     artifact{path: "generated/existing.json"},
			previous:     []byte("old existing"),
			previousMode: 0o640,
			existed:      true,
			backup:       "generated/existing.backup",
			backedUp:     true,
			installed:    true,
		},
		{
			artifact:  artifact{path: "generated/new.json"},
			existed:   false,
			installed: true,
		},
	}
	if err := rollbackReplacements(root, changes); err != nil {
		t.Fatalf("rollbackReplacements: %v", err)
	}
	if got := string(readFileForTest(t, filepath.Join(rootDirectory, "generated", "existing.json"))); got != "old existing" {
		t.Fatalf("restored existing = %q", got)
	}
	if _, err := os.Stat(filepath.Join(rootDirectory, "generated", "new.json")); !os.IsNotExist(err) {
		t.Fatalf("new output survived rollback: %v", err)
	}

	if err := os.WriteFile(filepath.Join(rootDirectory, "generated", "existing.json"), []byte("new again"), 0o644); err != nil {
		t.Fatal(err)
	}
	fallback := replacement{
		artifact:     artifact{path: "generated/existing.json"},
		previous:     []byte("recreated old"),
		previousMode: 0o600,
		existed:      true,
		canRecreate:  true,
		installed:    true,
	}
	if err := rollbackReplacements(root, []replacement{fallback}); err != nil {
		t.Fatalf("direct replacement rollback: %v", err)
	}
	if got := string(readFileForTest(t, filepath.Join(rootDirectory, "generated", "existing.json"))); got != "recreated old" {
		t.Fatalf("fallback restored = %q", got)
	}
	info, err := os.Stat(filepath.Join(rootDirectory, "generated", "existing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("fallback mode = %o", info.Mode().Perm())
	}
	assertNoTemporaryArtifacts(t, rootDirectory)
}

type treeEntry struct {
	Mode    fs.FileMode
	ModTime time.Time
	Data    string
}

func snapshotTree(t *testing.T, root string) map[string]treeEntry {
	t.Helper()
	result := make(map[string]treeEntry)
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		item := treeEntry{Mode: info.Mode(), ModTime: info.ModTime()}
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(name)
			if err != nil {
				return err
			}
			item.Data = string(data)
		}
		result[filepath.ToSlash(relative)] = item
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
