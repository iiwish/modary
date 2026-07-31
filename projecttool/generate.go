package projecttool

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/module"
)

const (
	// GraphSchemaVersion identifies the alpha generated Module graph format.
	GraphSchemaVersion = "modary.module-graph/v1alpha1"
	// CatalogSchemaVersion identifies the alpha generated Action catalog format.
	CatalogSchemaVersion = "modary.action-catalog/v1alpha1"
	// MaximumGeneratedArtifactBytes bounds both rendered and existing artifacts.
	MaximumGeneratedArtifactBytes = int64(64 << 20)
)

type projectRootGate struct {
	identity   fs.FileInfo
	semaphore  chan struct{}
	references int
}

var projectRootGates = struct {
	sync.Mutex
	entries []*projectRootGate
}{}

// acquireProjectRoot serializes operations only within this process. External
// writers must coordinate separately and are rejected when baseline checks can
// observe them.
func acquireProjectRoot(ctx context.Context, root string, identity fs.FileInfo) (func(), error) {
	if ctx == nil {
		return nil, ErrContextRequired
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if identity == nil || !identity.IsDir() {
		return nil, fmt.Errorf("project root identity is unavailable for %q", root)
	}
	projectRootGates.Lock()
	var gate *projectRootGate
	for _, candidate := range projectRootGates.entries {
		if os.SameFile(identity, candidate.identity) {
			gate = candidate
			break
		}
	}
	if gate == nil {
		gate = &projectRootGate{
			identity:  identity,
			semaphore: make(chan struct{}, 1),
		}
		projectRootGates.entries = append(projectRootGates.entries, gate)
	}
	gate.references++
	projectRootGates.Unlock()

	releaseReference := func() {
		projectRootGates.Lock()
		gate.references--
		if gate.references == 0 {
			for index, candidate := range projectRootGates.entries {
				if candidate == gate {
					projectRootGates.entries = append(projectRootGates.entries[:index], projectRootGates.entries[index+1:]...)
					break
				}
			}
		}
		projectRootGates.Unlock()
	}
	select {
	case gate.semaphore <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-gate.semaphore
			releaseReference()
			return nil, err
		}
		var once sync.Once
		return func() {
			once.Do(func() {
				<-gate.semaphore
				releaseReference()
			})
		}, nil
	case <-ctx.Done():
		releaseReference()
		return nil, ctx.Err()
	}
}

// Generation reports deterministic relative paths changed or already current.
type Generation struct {
	Written   []string `json:"written"`
	Unchanged []string `json:"unchanged"`
}

type artifact struct {
	path string
	data []byte
}

// GraphDocument is the alpha on-disk Module graph contract identified by
// GraphSchemaVersion.
type GraphDocument struct {
	SchemaVersion string                       `json:"schema_version"`
	Application   appkit.Metadata              `json:"application"`
	Modules       []ModuleInfo                 `json:"modules"`
	Edges         []module.GraphEdge           `json:"edges"`
	Order         []string                     `json:"order"`
	Provides      map[module.Capability]string `json:"provides"`
}

// CatalogDocument is the alpha on-disk Action contract identified by
// CatalogSchemaVersion.
type CatalogDocument struct {
	SchemaVersion string                `json:"schema_version"`
	Application   appkit.Metadata       `json:"application"`
	Actions       []action.CatalogEntry `json:"actions"`
}

func (project *Project) prepare(ctx context.Context, root *os.Root, definition appkit.Definition) (Snapshot, []artifact, error) {
	if ctx == nil {
		return Snapshot{}, nil, ErrContextRequired
	}
	if project == nil {
		return Snapshot{}, nil, fmt.Errorf("project is required")
	}
	if root == nil {
		return Snapshot{}, nil, fmt.Errorf("verified project root is required")
	}
	if err := project.validateFilesystemPathsContext(ctx, root); err != nil {
		return Snapshot{}, nil, err
	}
	snapshot, err := InspectContext(ctx, definition)
	if err != nil {
		return Snapshot{}, nil, err
	}
	if snapshot.Application != project.manifest.Application {
		return Snapshot{}, nil, fmt.Errorf("project manifest application identity does not match the supplied Definition")
	}
	artifacts, err := project.renderContext(ctx, snapshot)
	if err != nil {
		return Snapshot{}, nil, err
	}
	return snapshot, artifacts, nil
}

func (project *Project) render(snapshot Snapshot) ([]artifact, error) {
	return project.renderContext(context.Background(), snapshot)
}

func (project *Project) renderContext(ctx context.Context, snapshot Snapshot) ([]artifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	graphData, err := marshalGeneratedJSON(GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Application:   snapshot.Application,
		Modules:       append([]ModuleInfo(nil), snapshot.Modules...),
		Edges:         append([]module.GraphEdge(nil), snapshot.Graph.Edges...),
		Order:         append([]string(nil), snapshot.Graph.Order...),
		Provides:      cloneCapabilityOwners(snapshot.Graph.Provides),
	})
	if err != nil {
		return nil, fmt.Errorf("render Module graph: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	catalogData, err := marshalGeneratedJSON(CatalogDocument{
		SchemaVersion: CatalogSchemaVersion,
		Application:   snapshot.Application,
		Actions:       cloneCatalog(snapshot.Actions),
	})
	if err != nil {
		return nil, fmt.Errorf("render Action catalog: %w", err)
	}
	result := []artifact{
		{path: project.manifest.Outputs.Graph, data: graphData},
		{path: project.manifest.Outputs.Actions, data: catalogData},
	}
	if project.manifest.Outputs.TypeScript != "" {
		descriptors := make([]action.Descriptor, len(snapshot.Actions))
		for index, entry := range snapshot.Actions {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			descriptors[index] = entry.Descriptor
		}
		typescript, err := action.GenerateTypeScriptCatalog(descriptors)
		if err != nil {
			return nil, fmt.Errorf("render TypeScript Action contracts: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result = append(result, artifact{path: project.manifest.Outputs.TypeScript, data: append([]byte(nil), typescript...)})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].path < result[j].path })
	for _, item := range result {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if int64(len(item.data)) > MaximumGeneratedArtifactBytes {
			return nil, fmt.Errorf("generated artifact %s exceeds %d bytes", item.path, MaximumGeneratedArtifactBytes)
		}
	}
	return result, nil
}

func marshalGeneratedJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func cloneCapabilityOwners(values map[module.Capability]string) map[module.Capability]string {
	clone := make(map[module.Capability]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

// Generate validates and renders the complete batch before writing. Every
// changed file is replaced with one atomic rename, but the generated set is not
// a filesystem-wide transaction. Calls in this process are serialized per root
// and a commit failure attempts to roll already replaced files back in process.
func (project *Project) Generate(definition appkit.Definition) (Generation, error) {
	return project.GenerateContext(context.Background(), definition)
}

// GenerateContext is the cancelable form of Generate. Cancellation before the
// first replacement removes all prepared files and leaves every configured
// artifact at its baseline state.
func (project *Project) GenerateContext(ctx context.Context, definition appkit.Definition) (Generation, error) {
	if ctx == nil {
		return Generation{}, ErrContextRequired
	}
	if project == nil {
		return Generation{}, fmt.Errorf("project is required")
	}
	if err := ctx.Err(); err != nil {
		return Generation{}, err
	}
	release, err := acquireProjectRoot(ctx, project.root, project.rootIdentity)
	if err != nil {
		return Generation{}, err
	}
	defer release()
	root, err := project.openVerifiedRoot(ctx)
	if err != nil {
		return Generation{}, err
	}
	defer root.Close()
	_, artifacts, err := project.prepare(ctx, root, definition)
	if err != nil {
		return Generation{}, err
	}
	if err := project.validateFilesystemPathsContext(ctx, root); err != nil {
		return Generation{}, err
	}

	baselines := make([]artifactBaseline, 0, len(artifacts))
	changes := make([]replacement, 0, len(artifacts))
	result := Generation{Written: make([]string, 0), Unchanged: make([]string, 0)}
	for _, item := range artifacts {
		if err := ctx.Err(); err != nil {
			return Generation{}, err
		}
		current, mode, exists, err := readArtifactContext(ctx, root, item)
		if err != nil {
			return Generation{}, err
		}
		if exists && bytes.Equal(current, item.data) {
			result.Unchanged = append(result.Unchanged, item.path)
			baselines = append(baselines, artifactBaseline{artifact: item, data: current, mode: mode, exists: true})
			continue
		}
		baselines = append(baselines, artifactBaseline{artifact: item, data: current, mode: mode, exists: exists})
		changes = append(changes, replacement{
			artifact: item, previous: current, previousMode: mode,
			existed: exists, canRecreate: exists,
		})
	}
	if len(changes) == 0 {
		if err := project.verifyRootPathBinding(root); err != nil {
			return Generation{}, err
		}
		if err := ctx.Err(); err != nil {
			return Generation{}, err
		}
		return result, nil
	}

	createdDirectories := make([]string, 0)
	cleanupPrepared := func() error {
		var cleanupErr error
		for _, change := range changes {
			if change.temporary != "" {
				if err := root.Remove(change.temporary); err != nil && !errors.Is(err, fs.ErrNotExist) {
					cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove temporary artifact %s: %w", change.temporary, err))
				}
			}
			if change.backup != "" {
				if err := root.Remove(change.backup); err != nil && !errors.Is(err, fs.ErrNotExist) {
					cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove backup artifact %s: %w", change.backup, err))
				}
			}
		}
		removeEmptyDirectories(root, createdDirectories)
		return cleanupErr
	}
	for index := range changes {
		if err := ctx.Err(); err != nil {
			return Generation{}, errors.Join(err, cleanupPrepared())
		}
		created, err := ensureArtifactDirectoriesContext(ctx, root, path.Dir(changes[index].artifact.path))
		createdDirectories = append(createdDirectories, created...)
		if err != nil {
			return Generation{}, errors.Join(err, cleanupPrepared())
		}
		temporary, file, err := createSiblingTemporaryContext(ctx, root, changes[index].artifact.path, ".tmp")
		if err != nil {
			return Generation{}, errors.Join(err, cleanupPrepared())
		}
		changes[index].temporary = temporary
		writeErr := writeGeneratedContext(ctx, file, changes[index].artifact.data, 0o644)
		if writeErr != nil {
			return Generation{}, errors.Join(
				fmt.Errorf("prepare generated artifact %s: %w", changes[index].artifact.path, writeErr),
				cleanupPrepared(),
			)
		}
	}

	if err := project.validateFilesystemPathsContext(ctx, root); err != nil {
		return Generation{}, errors.Join(err, cleanupPrepared())
	}
	if err := project.verifyRootPathBinding(root); err != nil {
		return Generation{}, errors.Join(err, cleanupPrepared())
	}
	for _, baseline := range baselines {
		if err := ctx.Err(); err != nil {
			return Generation{}, errors.Join(err, cleanupPrepared())
		}
		current, mode, exists, err := readArtifactContext(ctx, root, baseline.artifact)
		if err != nil || exists != baseline.exists || !bytes.Equal(current, baseline.data) {
			cleanupErr := cleanupPrepared()
			if err != nil {
				return Generation{}, errors.Join(err, cleanupErr)
			}
			return Generation{}, errors.Join(fmt.Errorf("generated artifact %s changed concurrently", baseline.artifact.path), cleanupErr)
		}
		if exists && mode != baseline.mode {
			return Generation{}, errors.Join(
				fmt.Errorf("generated artifact %s changed concurrently", baseline.artifact.path),
				cleanupPrepared(),
			)
		}
	}
	if err := project.verifyRootPathBinding(root); err != nil {
		return Generation{}, errors.Join(err, cleanupPrepared())
	}
	if err := ctx.Err(); err != nil {
		return Generation{}, errors.Join(err, cleanupPrepared())
	}

	// No cancellation checks occur after this point. Each rename is atomic; the
	// in-process rollback completes before another same-root operation proceeds.
	for index := range changes {
		if err := root.Rename(changes[index].temporary, changes[index].artifact.path); err != nil {
			rollbackErr := rollbackAndSyncReplacements(root, changes[:index])
			return Generation{}, errors.Join(
				fmt.Errorf("replace generated artifact %s: %w", changes[index].artifact.path, err),
				rollbackErr,
				cleanupPrepared(),
			)
		}
		changes[index].temporary = ""
		changes[index].installed = true
	}

	if err := syncArtifactDirectories(root, changes); err != nil {
		rollbackErr := rollbackAndSyncReplacements(root, changes)
		return Generation{}, errors.Join(
			fmt.Errorf("sync generated artifact directories: %w", err),
			rollbackErr,
			cleanupPrepared(),
		)
	}
	if err := project.verifyRootPathBinding(root); err != nil {
		rollbackErr := rollbackAndSyncReplacements(root, changes)
		return Generation{}, errors.Join(err, rollbackErr, cleanupPrepared())
	}

	var cleanupErr error
	for index := range changes {
		if changes[index].backup != "" {
			if err := root.Remove(changes[index].backup); err != nil && !errors.Is(err, fs.ErrNotExist) {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove generated backup: %w", err))
			} else {
				changes[index].backup = ""
			}
		}
	}
	if cleanupErr != nil {
		rollbackErr := rollbackAndSyncReplacements(root, changes)
		return Generation{}, errors.Join(cleanupErr, rollbackErr, cleanupPrepared())
	}
	for _, change := range changes {
		result.Written = append(result.Written, change.artifact.path)
	}
	return result, nil
}

type artifactBaseline struct {
	artifact artifact
	data     []byte
	mode     fs.FileMode
	exists   bool
}

type replacement struct {
	artifact     artifact
	previous     []byte
	previousMode fs.FileMode
	existed      bool
	canRecreate  bool
	temporary    string
	backup       string
	backedUp     bool
	installed    bool
}

func readArtifact(root *os.Root, expected artifact) ([]byte, fs.FileMode, bool, error) {
	return readArtifactContext(context.Background(), root, expected)
}

func readArtifactContext(ctx context.Context, root *os.Root, expected artifact) ([]byte, fs.FileMode, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, false, err
	}
	before, err := root.Lstat(expected.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("inspect generated artifact %s: %w", expected.path, err)
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return nil, 0, false, fmt.Errorf("generated artifact %s must be a regular non-symlink file", expected.path)
	}
	if before.Size() > MaximumGeneratedArtifactBytes {
		return nil, 0, false, fmt.Errorf("generated artifact %s exceeds %d bytes", expected.path, MaximumGeneratedArtifactBytes)
	}
	file, err := root.Open(expected.path)
	if err != nil {
		return nil, 0, false, fmt.Errorf("open generated artifact %s: %w", expected.path, err)
	}
	openedBefore, statErr := file.Stat()
	data, readErr := readAllContext(ctx, &io.LimitedReader{R: file, N: MaximumGeneratedArtifactBytes + 1})
	openedAfter, restatErr := file.Stat()
	closeErr := file.Close()
	after, lstatErr := root.Lstat(expected.path)
	if statErr != nil || readErr != nil || restatErr != nil || closeErr != nil || lstatErr != nil {
		return nil, 0, false, fmt.Errorf(
			"read generated artifact %s consistently: %w",
			expected.path,
			errors.Join(statErr, readErr, restatErr, closeErr, lstatErr),
		)
	}
	if int64(len(data)) > MaximumGeneratedArtifactBytes {
		return nil, 0, false, fmt.Errorf("generated artifact %s exceeds %d bytes", expected.path, MaximumGeneratedArtifactBytes)
	}
	if after.Mode()&os.ModeSymlink != 0 || !after.Mode().IsRegular() ||
		!sameFileState(before, openedBefore) || !sameFileState(openedBefore, openedAfter) || !sameFileState(openedAfter, after) {
		return nil, 0, false, fmt.Errorf("generated artifact %s changed while it was read", expected.path)
	}
	return data, openedAfter.Mode(), true, nil
}

func ensureArtifactDirectories(root *os.Root, directory string) ([]string, error) {
	return ensureArtifactDirectoriesContext(context.Background(), root, directory)
}

func ensureArtifactDirectoriesContext(ctx context.Context, root *os.Root, directory string) ([]string, error) {
	if directory == "." || directory == "" {
		return nil, nil
	}
	created := make([]string, 0)
	components := strings.Split(directory, "/")
	for index := range components {
		if err := ctx.Err(); err != nil {
			return created, err
		}
		current := strings.Join(components[:index+1], "/")
		info, err := root.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			if mkdirErr := root.Mkdir(current, 0o755); mkdirErr != nil {
				if errors.Is(mkdirErr, fs.ErrExist) {
					concurrentInfo, inspectErr := root.Lstat(current)
					if inspectErr == nil && concurrentInfo.Mode()&os.ModeSymlink == 0 && concurrentInfo.IsDir() {
						continue
					}
					if inspectErr != nil {
						return created, fmt.Errorf("inspect concurrently created directory %s: %w", current, inspectErr)
					}
					return created, fmt.Errorf("concurrently created path %s must be a non-symlink directory", current)
				}
				return created, fmt.Errorf("create generated directory %s: %w", current, mkdirErr)
			}
			created = append(created, current)
			continue
		}
		if err != nil {
			return created, fmt.Errorf("inspect generated directory %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return created, fmt.Errorf("generated directory %s must be a non-symlink directory", current)
		}
	}
	return created, nil
}

func createSiblingTemporary(root *os.Root, target, suffix string) (string, *os.File, error) {
	return createSiblingTemporaryContext(context.Background(), root, target, suffix)
}

func createSiblingTemporaryContext(ctx context.Context, root *os.Root, target, suffix string) (string, *os.File, error) {
	for attempts := 0; attempts < 32; attempts++ {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		candidate, err := randomSiblingName(target, suffix)
		if err != nil {
			return "", nil, err
		}
		file, err := root.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", nil, fmt.Errorf("create temporary artifact beside %s: %w", target, err)
		}
		return candidate, file, nil
	}
	return "", nil, fmt.Errorf("allocate temporary artifact beside %s", target)
}

func randomSiblingName(target, suffix string) (string, error) {
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate temporary name: %w", err)
	}
	base := path.Base(target)
	if len(base) > 80 {
		for len(base) > 80 {
			_, size := utf8.DecodeLastRuneInString(base)
			base = base[:len(base)-size]
		}
	}
	return path.Join(path.Dir(target), ".modary-"+base+"-"+hex.EncodeToString(random[:])+suffix), nil
}

func writeGenerated(file *os.File, data []byte, mode fs.FileMode) error {
	return writeGeneratedContext(context.Background(), file, data, mode)
}

func writeGeneratedContext(ctx context.Context, file *os.File, data []byte, mode fs.FileMode) error {
	if file == nil {
		return fmt.Errorf("temporary artifact is unavailable")
	}
	var result error
	if err := ctx.Err(); err != nil {
		result = errors.Join(result, err)
	} else if err := file.Chmod(mode); err != nil {
		result = errors.Join(result, err)
	} else {
		for offset := 0; offset < len(data); {
			if err := ctx.Err(); err != nil {
				result = errors.Join(result, err)
				break
			}
			end := offset + 32<<10
			if end > len(data) {
				end = len(data)
			}
			written, err := file.Write(data[offset:end])
			if err != nil {
				result = errors.Join(result, err)
				break
			}
			if written != end-offset {
				result = errors.Join(result, io.ErrShortWrite)
				break
			}
			offset = end
		}
		if result == nil {
			if err := ctx.Err(); err != nil {
				result = errors.Join(result, err)
			} else if err := file.Sync(); err != nil {
				result = errors.Join(result, err)
			}
		}
	}
	if err := file.Close(); err != nil {
		result = errors.Join(result, err)
	}
	return result
}

func rollbackReplacements(root *os.Root, changes []replacement) error {
	var result error
	for index := len(changes) - 1; index >= 0; index-- {
		change := changes[index]
		if !change.installed && !change.backedUp {
			continue
		}
		if !change.existed {
			if change.installed {
				if err := root.Remove(change.artifact.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
					result = errors.Join(result, fmt.Errorf("remove replacement %s: %w", change.artifact.path, err))
				}
			}
			continue
		}

		if change.backedUp && change.backup != "" {
			restoreErr := root.Rename(change.backup, change.artifact.path)
			if restoreErr == nil {
				continue
			}
			if !errors.Is(restoreErr, fs.ErrNotExist) || !change.canRecreate {
				result = errors.Join(result, fmt.Errorf("restore generated artifact %s: %w", change.artifact.path, restoreErr))
				continue
			}
		} else if !change.canRecreate {
			result = errors.Join(result, fmt.Errorf("restore generated artifact %s: prior content is unavailable", change.artifact.path))
			continue
		}

		temporary, file, restoreErr := createSiblingTemporary(root, change.artifact.path, ".restore")
		if restoreErr == nil {
			restoreErr = writeGenerated(file, change.previous, change.previousMode)
		}
		if restoreErr == nil {
			restoreErr = root.Rename(temporary, change.artifact.path)
		}
		if restoreErr != nil {
			if temporary != "" {
				_ = root.Remove(temporary)
			}
			result = errors.Join(result, fmt.Errorf("recreate generated artifact %s: %w", change.artifact.path, restoreErr))
		}
	}
	return result
}

func rollbackAndSyncReplacements(root *os.Root, changes []replacement) error {
	rollbackErr := rollbackReplacements(root, changes)
	syncErr := syncArtifactDirectories(root, changes)
	return errors.Join(rollbackErr, syncErr)
}

func removeEmptyDirectories(root *os.Root, directories []string) {
	for index := len(directories) - 1; index >= 0; index-- {
		_ = root.Remove(directories[index])
	}
}

func syncArtifactDirectories(root *os.Root, changes []replacement) error {
	directories := make(map[string]struct{})
	for _, change := range changes {
		directories[path.Dir(change.artifact.path)] = struct{}{}
	}
	names := make([]string, 0, len(directories))
	for name := range directories {
		names = append(names, name)
	}
	sort.Strings(names)
	var result error
	for _, name := range names {
		file, err := root.Open(name)
		if err != nil {
			result = errors.Join(result, fmt.Errorf("open generated directory %s for sync: %w", name, err))
			continue
		}
		if err := file.Sync(); err != nil && runtime.GOOS != "windows" {
			result = errors.Join(result, fmt.Errorf("sync generated directory %s: %w", name, err))
		}
		if err := file.Close(); err != nil {
			result = errors.Join(result, fmt.Errorf("close generated directory %s: %w", name, err))
		}
	}
	return result
}

// Check compares every expected artifact without creating a directory or file.
// Drift entries are sorted by path.
func (project *Project) Check(definition appkit.Definition) ([]Drift, error) {
	return project.CheckContext(context.Background(), definition)
}

// CheckContext is the cancelable form of Check.
func (project *Project) CheckContext(ctx context.Context, definition appkit.Definition) ([]Drift, error) {
	if ctx == nil {
		return nil, ErrContextRequired
	}
	if project == nil {
		return nil, fmt.Errorf("project is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	release, err := acquireProjectRoot(ctx, project.root, project.rootIdentity)
	if err != nil {
		return nil, err
	}
	defer release()
	root, err := project.openVerifiedRoot(ctx)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	_, artifacts, err := project.prepare(ctx, root, definition)
	if err != nil {
		return nil, err
	}
	drift, err := checkArtifactsContext(ctx, root, artifacts)
	if err != nil {
		return nil, err
	}
	if err := project.verifyRootPathBinding(root); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return drift, nil
}

func checkArtifacts(root *os.Root, artifacts []artifact) ([]Drift, error) {
	return checkArtifactsContext(context.Background(), root, artifacts)
}

func checkArtifactsContext(ctx context.Context, root *os.Root, artifacts []artifact) ([]Drift, error) {
	drift := make([]Drift, 0)
	for _, item := range artifacts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		current, _, exists, err := readArtifactContext(ctx, root, item)
		if err != nil {
			return nil, err
		}
		if !exists {
			drift = append(drift, Drift{Path: item.path, Status: DriftMissing})
		} else if !bytes.Equal(current, item.data) {
			drift = append(drift, Drift{Path: item.path, Status: DriftDifferent})
		}
	}
	return drift, ctx.Err()
}
