package projecttool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/iiwish/modary/appkit"
)

// BuildOptions receives Go compiler output. Nil writers select the process
// standard streams; typed nil writers are rejected before compiler invocation.
// Writers are trusted, cooperative dependencies: Write must return. Build can
// bound a canceled compiler and inherited compiler pipes, but cannot interrupt
// an arbitrary io.Writer blocked inside its own Write method.
type BuildOptions struct {
	Stdout io.Writer
	Stderr io.Writer
}

// BuildResult identifies the installed consumer binary.
type BuildResult struct {
	Output string `json:"output"`
}

// Build verifies the supplied Definition, requires every generated artifact to
// be current, and invokes only "go build" for the configured consumer package.
// Ambient GOFLAGS, GOENV, and GOWORK are disabled so they cannot inject tool
// execution or replace the verified consumer module graph.
// The compiler writes into a private staging directory outside the project. A
// validated output is copied through the verified project Root and installed
// with one sibling rename, which is atomic only where the host filesystem
// guarantees rename atomicity. An existing live binary is never moved out of
// the way first. Same-root builds are serialized within this process and
// post-install failures attempt rollback from a prepared sibling copy.
func (project *Project) Build(ctx context.Context, definition appkit.Definition, options BuildOptions) (result BuildResult, resultErr error) {
	if ctx == nil {
		return BuildResult{}, ErrContextRequired
	}
	if err := ctx.Err(); err != nil {
		return BuildResult{}, err
	}
	if project == nil {
		return BuildResult{}, fmt.Errorf("project is required")
	}
	if err := validateSecureBuildPlatform(); err != nil {
		return BuildResult{}, err
	}
	stdout, stderr, err := newBuildWriters(options)
	if err != nil {
		return BuildResult{}, err
	}

	release, err := acquireProjectRoot(ctx, project.root, project.rootIdentity)
	if err != nil {
		return BuildResult{}, err
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return BuildResult{}, err
	}
	root, err := project.openVerifiedRoot(ctx)
	if err != nil {
		return BuildResult{}, err
	}
	defer root.Close()
	_, artifacts, err := project.prepare(ctx, root, definition)
	if err != nil {
		return BuildResult{}, err
	}
	drift, err := checkArtifactsContext(ctx, root, artifacts)
	if err != nil {
		return BuildResult{}, err
	}
	if len(drift) != 0 {
		return BuildResult{}, &DriftError{Items: append([]Drift(nil), drift...)}
	}
	if err := validateConfiguredDirectoryPathContext(ctx, root, strings.TrimPrefix(project.manifest.Build.Package, "./"), true); err != nil {
		return BuildResult{}, fmt.Errorf("validate build package: %w", err)
	}

	target := project.manifest.Build.Output
	baseline, err := inspectBuildTarget(root, target)
	if err != nil {
		return BuildResult{}, err
	}
	staging, err := newBuildStaging(project.rootIdentity)
	if err != nil {
		return BuildResult{}, err
	}
	stagingNeedsClose := true
	defer func() {
		if stagingNeedsClose {
			resultErr = errors.Join(resultErr, staging.Close())
		}
	}()
	createdDirectories := []string(nil)
	temporary := ""
	cleanupDirectories := func() { removeEmptyDirectories(root, createdDirectories) }
	cleanupBuild := func() error {
		var cleanupErr error
		if temporary != "" {
			if err := root.Remove(temporary); err != nil && !errors.Is(err, fs.ErrNotExist) {
				cleanupErr = fmt.Errorf("remove temporary build output %s: %w", temporary, err)
			}
		}
		cleanupDirectories()
		return cleanupErr
	}

	if err := ctx.Err(); err != nil {
		return BuildResult{}, errors.Join(err, cleanupBuild())
	}
	if err := project.verifyRootPathBinding(root); err != nil {
		return BuildResult{}, errors.Join(err, cleanupBuild())
	}
	if err := staging.validatePathBinding(); err != nil {
		return BuildResult{}, errors.Join(err, cleanupBuild())
	}
	command := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-buildvcs=false", "-trimpath", "-o", staging.OutputPath(), project.manifest.Build.Package)
	command.Dir = project.root
	command.Env = goBuildEnvironment(os.Environ(), staging.parentDirectory)
	command.Stdout = stdout
	command.Stderr = stderr
	command.WaitDelay = buildCommandWaitDelay
	configureBuildCommand(command)
	startErr := command.Start()
	var commandErr error
	var processCleanupErr error
	if startErr != nil {
		commandErr = startErr
	} else {
		commandErr, processCleanupErr = awaitBuildCommand(ctx, command)
	}
	writerErr := errors.Join(stdout.Err(), stderr.Err())
	bindingErr := project.verifyRootPathBinding(root)
	if commandErr != nil || processCleanupErr != nil || writerErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return BuildResult{}, errors.Join(ctxErr, processCleanupErr, writerErr, bindingErr, cleanupBuild())
		}
		return BuildResult{}, errors.Join(
			fmt.Errorf("build Go package %s: %w", project.manifest.Build.Package, errors.Join(commandErr, processCleanupErr, writerErr, bindingErr)),
			cleanupBuild(),
		)
	}
	if bindingErr != nil {
		return BuildResult{}, errors.Join(bindingErr, cleanupBuild())
	}
	stagedOutput, err := staging.OpenOutput()
	if err != nil {
		return BuildResult{}, errors.Join(err, cleanupBuild())
	}
	if err := ctx.Err(); err != nil {
		return BuildResult{}, errors.Join(err, stagedOutput.Close(), cleanupBuild())
	}
	if err := project.validateFilesystemPathsContext(ctx, root); err != nil {
		return BuildResult{}, errors.Join(err, stagedOutput.Close(), cleanupBuild())
	}
	if err := project.verifyRootPathBinding(root); err != nil {
		return BuildResult{}, errors.Join(err, stagedOutput.Close(), cleanupBuild())
	}
	if err := validateConfiguredDirectoryPathContext(ctx, root, strings.TrimPrefix(project.manifest.Build.Package, "./"), true); err != nil {
		return BuildResult{}, errors.Join(fmt.Errorf("validate build package after Go build: %w", err), stagedOutput.Close(), cleanupBuild())
	}
	secondDrift, err := checkArtifactsContext(ctx, root, artifacts)
	if err != nil {
		return BuildResult{}, errors.Join(err, stagedOutput.Close(), cleanupBuild())
	}
	if len(secondDrift) != 0 {
		return BuildResult{}, errors.Join(
			&DriftError{Items: append([]Drift(nil), secondDrift...)},
			stagedOutput.Close(),
			cleanupBuild(),
		)
	}
	if err := baseline.matches(root, target); err != nil {
		return BuildResult{}, errors.Join(err, stagedOutput.Close(), cleanupBuild())
	}
	if err := project.verifyRootPathBinding(root); err != nil {
		return BuildResult{}, errors.Join(err, stagedOutput.Close(), cleanupBuild())
	}
	if err := ctx.Err(); err != nil {
		return BuildResult{}, errors.Join(err, stagedOutput.Close(), cleanupBuild())
	}
	createdDirectories, err = ensureArtifactDirectoriesContext(ctx, root, path.Dir(target))
	if err != nil {
		return BuildResult{}, errors.Join(err, stagedOutput.Close(), cleanupBuild())
	}
	temporary, err = copyStagedBuildOutput(ctx, root, target, stagedOutput)
	if err != nil {
		return BuildResult{}, errors.Join(err, cleanupBuild())
	}
	stagingErr := staging.Close()
	stagingNeedsClose = false
	if stagingErr != nil {
		return BuildResult{}, errors.Join(stagingErr, cleanupBuild())
	}
	if err := ctx.Err(); err != nil {
		return BuildResult{}, errors.Join(err, cleanupBuild())
	}

	// The final rename is the commit point. Everything before it remains
	// cancelable and leaves the live pathname untouched; everything after it
	// deliberately completes or rolls back without consulting the context.
	change := replacement{
		artifact:     artifact{path: target},
		existed:      baseline.exists,
		previousMode: baseline.mode,
		temporary:    temporary,
	}
	if baseline.exists {
		backup, err := copyBuildTargetToSibling(ctx, root, target, baseline)
		if err != nil {
			return BuildResult{}, errors.Join(err, cleanupBuild())
		}
		change.backup = backup
		change.backedUp = true
	}
	cleanupPreCommit := func() error {
		return errors.Join(removeUnusedBuildBackup(root, change.backup), cleanupBuild())
	}
	if err := ctx.Err(); err != nil {
		return BuildResult{}, errors.Join(err, cleanupPreCommit())
	}
	if err := project.verifyRootPathBinding(root); err != nil {
		return BuildResult{}, errors.Join(err, cleanupPreCommit())
	}
	if err := baseline.matches(root, target); err != nil {
		return BuildResult{}, errors.Join(err, cleanupPreCommit())
	}
	if err := ctx.Err(); err != nil {
		return BuildResult{}, errors.Join(err, cleanupPreCommit())
	}
	if err := root.Rename(temporary, target); err != nil {
		return BuildResult{}, errors.Join(fmt.Errorf("replace build output %s: %w", target, err), cleanupPreCommit())
	}
	change.temporary = ""
	temporary = ""
	change.installed = true
	if err := syncArtifactDirectories(root, []replacement{change}); err != nil {
		rollbackErr := rollbackBuildReplacement(root, change)
		return BuildResult{}, errors.Join(fmt.Errorf("sync build output directory: %w", err), rollbackErr, cleanupBuild())
	}
	if err := project.verifyRootPathBinding(root); err != nil {
		rollbackErr := rollbackBuildReplacement(root, change)
		return BuildResult{}, errors.Join(err, rollbackErr, cleanupBuild())
	}
	if change.backup != "" {
		if err := root.Remove(change.backup); err != nil && !errors.Is(err, fs.ErrNotExist) {
			rollbackErr := rollbackBuildReplacement(root, change)
			return BuildResult{}, errors.Join(fmt.Errorf("remove build output backup: %w", err), rollbackErr, cleanupBuild())
		}
	}
	return BuildResult{Output: target}, nil
}

func awaitBuildCommand(ctx context.Context, command *exec.Cmd) (commandErr, cleanupErr error) {
	// Observe the group leader without reaping it. While that zombie still
	// reserves its PID (and therefore the process-group ID established in
	// configureBuildCommand), the group can be terminated without racing a
	// recycled PGID. Cmd.Wait then performs the one authoritative reap and
	// completes os/exec's stream-copy lifecycle.
	observeErr := waitBuildCommandExit(command)
	cleanupErr = cleanupBuildCommand(command)
	waitErr := command.Wait()
	// ErrWaitDelay means the leader itself succeeded but an inherited pipe
	// needed the configured close backstop. Group cleanup has already run while
	// the PID/PGID was reserved, so this is not a compiler failure; explicit
	// writer failures are collected separately by Build.
	if observeErr == nil && cleanupErr == nil && ctx.Err() == nil && errors.Is(waitErr, exec.ErrWaitDelay) {
		waitErr = nil
	}
	if observeErr != nil {
		return errors.Join(fmt.Errorf("observe Go build process exit: %w", observeErr), waitErr), cleanupErr
	}
	return waitErr, cleanupErr
}

func rollbackBuildReplacement(root *os.Root, change replacement) error {
	return rollbackAndSyncReplacements(root, []replacement{change})
}

func goBuildEnvironment(environment []string, temporaryDirectory string) []string {
	result := make([]string, 0, len(environment)+7)
	for _, item := range environment {
		name := item
		if index := strings.IndexByte(name, '='); index >= 0 {
			name = name[:index]
		}
		if strings.EqualFold(name, "GOFLAGS") || strings.EqualFold(name, "GOENV") ||
			strings.EqualFold(name, "GOWORK") || strings.EqualFold(name, "GO111MODULE") ||
			strings.EqualFold(name, "GOTOOLCHAIN") || strings.EqualFold(name, "GOROOT") ||
			strings.EqualFold(name, "TMPDIR") || strings.EqualFold(name, "GOTMPDIR") {
			continue
		}
		result = append(result, item)
	}
	return append(result,
		"GOFLAGS=", "GOENV=off", "GOWORK=off", "GO111MODULE=on", "GOTOOLCHAIN=local",
		"TMPDIR="+temporaryDirectory, "GOTMPDIR="+temporaryDirectory,
	)
}

const (
	buildStagingOutputName = "consumer-binary"
	buildCommandWaitDelay  = 2 * time.Second
)

type buildStaging struct {
	projectIdentity fs.FileInfo
	parentDirectory string
	parentInfo      fs.FileInfo
	parent          *os.Root
	parentSecurity  *os.File
	ancestors       []buildStagingAncestor
	name            string
	directory       string
	info            fs.FileInfo
	security        *os.File
	root            *os.Root
	once            sync.Once
	err             error
}

type buildStagingAncestor struct {
	directory string
	info      fs.FileInfo
	security  *os.File
}

func newBuildStaging(projectIdentity fs.FileInfo) (staging *buildStaging, result error) {
	if projectIdentity == nil || !projectIdentity.IsDir() {
		return nil, fmt.Errorf("project root identity is unavailable")
	}
	temporaryRoot, err := filepath.Abs(os.TempDir())
	if err != nil {
		return nil, fmt.Errorf("resolve operating system temporary directory: %w", err)
	}
	temporaryRoot, err = filepath.EvalSymlinks(temporaryRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve operating system temporary directory symlinks: %w", err)
	}
	if err := requireBuildStagingOutsideProject(temporaryRoot, projectIdentity); err != nil {
		return nil, err
	}
	parent, err := os.OpenRoot(temporaryRoot)
	if err != nil {
		return nil, fmt.Errorf("open operating system temporary directory: %w", err)
	}
	parentSecurity, err := parent.Open(".")
	if err != nil {
		return nil, errors.Join(fmt.Errorf("open operating system temporary directory security handle: %w", err), parent.Close())
	}
	parentInfo, parentStatErr := parent.Stat(".")
	parentSecurityInfo, parentSecurityStatErr := parentSecurity.Stat()
	currentParent, currentParentErr := os.Lstat(temporaryRoot)
	if parentStatErr != nil || parentSecurityStatErr != nil || currentParentErr != nil || parentInfo == nil ||
		parentSecurityInfo == nil || currentParent == nil || !parentInfo.IsDir() || !parentSecurityInfo.IsDir() ||
		currentParent.Mode()&os.ModeSymlink != 0 || !os.SameFile(parentInfo, parentSecurityInfo) || !os.SameFile(parentInfo, currentParent) {
		if parentStatErr == nil && parentSecurityStatErr == nil && currentParentErr == nil {
			parentStatErr = fmt.Errorf("operating system temporary directory changed identity")
		}
		return nil, errors.Join(
			fmt.Errorf("verify operating system temporary directory: %w", errors.Join(parentStatErr, parentSecurityStatErr, currentParentErr)),
			parentSecurity.Close(),
			parent.Close(),
		)
	}
	if err := validateBuildStagingParentProtection(parentSecurity, parentSecurityInfo); err != nil {
		return nil, errors.Join(err, parentSecurity.Close(), parent.Close())
	}
	ancestors, err := openBuildStagingAncestors(temporaryRoot)
	if err != nil {
		return nil, errors.Join(err, parentSecurity.Close(), parent.Close())
	}
	closeParentHandles := func() error {
		return errors.Join(parentSecurity.Close(), parent.Close(), closeBuildStagingAncestors(ancestors))
	}
	name := ""
	for attempts := 0; attempts < 32; attempts++ {
		candidate, randomErr := randomSiblingName("build", ".stage")
		if randomErr != nil {
			return nil, errors.Join(randomErr, closeParentHandles())
		}
		if mkdirErr := parent.Mkdir(candidate, 0o700); errors.Is(mkdirErr, fs.ErrExist) {
			continue
		} else if mkdirErr != nil {
			return nil, errors.Join(fmt.Errorf("create private build staging directory: %w", mkdirErr), closeParentHandles())
		}
		name = candidate
		break
	}
	if name == "" {
		return nil, errors.Join(fmt.Errorf("allocate private build staging directory"), closeParentHandles())
	}
	directory := filepath.Join(temporaryRoot, filepath.FromSlash(name))
	createdInfo, err := parent.Lstat(name)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("inspect created build staging directory: %w", err), closeParentHandles())
	}
	cleanup := true
	var security *os.File
	var root *os.Root
	defer func() {
		if cleanup {
			if root != nil {
				result = errors.Join(result, root.Close())
			}
			if security != nil {
				result = errors.Join(result, security.Close())
			}
			result = errors.Join(result, removeBuildStagingFromParent(parent, name, createdInfo), closeParentHandles())
		}
	}()
	if !createdInfo.IsDir() || createdInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("created build staging path is not a directory")
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !os.SameFile(createdInfo, info) {
		if err == nil {
			err = fmt.Errorf("build staging path is not a private owner-only directory")
		}
		return nil, fmt.Errorf("inspect build staging directory: %w", err)
	}
	if err := requireBuildStagingOutsideProject(directory, projectIdentity); err != nil {
		return nil, err
	}
	security, err = parent.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open build staging security handle: %w", err)
	}
	if err := security.Chmod(0o700); err != nil {
		return nil, fmt.Errorf("protect build staging directory: %w", err)
	}
	securityInfo, securityStatErr := security.Stat()
	if securityStatErr != nil || securityInfo == nil || !os.SameFile(info, securityInfo) {
		if securityStatErr == nil {
			securityStatErr = fmt.Errorf("build staging security handle has a different identity")
		}
		return nil, fmt.Errorf("verify build staging security handle: %w", securityStatErr)
	}
	if err := validateBuildStagingProtection(security, securityInfo); err != nil {
		return nil, err
	}
	root, err = parent.OpenRoot(name)
	if err != nil {
		return nil, fmt.Errorf("open build staging directory: %w", err)
	}
	opened, statErr := root.Stat(".")
	if statErr != nil || opened == nil || !opened.IsDir() || !os.SameFile(info, opened) {
		if statErr == nil {
			statErr = fmt.Errorf("opened build staging Root has a different identity")
		}
		return nil, fmt.Errorf("verify opened build staging directory: %w", statErr)
	}
	staging = &buildStaging{
		projectIdentity: projectIdentity,
		parentDirectory: temporaryRoot,
		parentInfo:      parentInfo,
		parent:          parent,
		parentSecurity:  parentSecurity,
		ancestors:       ancestors,
		name:            name,
		directory:       directory,
		info:            info,
		security:        security,
		root:            root,
	}
	if err := staging.validatePathBinding(); err != nil {
		return nil, err
	}
	cleanup = false
	return staging, nil
}

func openBuildStagingAncestors(parentDirectory string) ([]buildStagingAncestor, error) {
	if !filepath.IsAbs(parentDirectory) || filepath.Clean(parentDirectory) != parentDirectory {
		return nil, fmt.Errorf("operating system temporary directory must be canonical")
	}
	canonical, err := filepath.EvalSymlinks(parentDirectory)
	if err != nil || filepath.Clean(canonical) != parentDirectory {
		return nil, fmt.Errorf("operating system temporary directory must be canonical")
	}
	ancestors := make([]buildStagingAncestor, 0, 16)
	for current := filepath.Dir(parentDirectory); current != parentDirectory; current = filepath.Dir(current) {
		security, err := os.Open(current)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("open build staging ancestor %s: %w", current, err), closeBuildStagingAncestors(ancestors))
		}
		info, statErr := security.Stat()
		pathInfo, pathErr := os.Lstat(current)
		if statErr != nil || pathErr != nil || info == nil || pathInfo == nil || !info.IsDir() ||
			pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, pathInfo) {
			if statErr == nil && pathErr == nil {
				statErr = fmt.Errorf("build staging ancestor changed identity")
			}
			return nil, errors.Join(
				fmt.Errorf("verify build staging ancestor %s: %w", current, errors.Join(statErr, pathErr)),
				security.Close(),
				closeBuildStagingAncestors(ancestors),
			)
		}
		if err := validateBuildStagingParentProtection(security, info); err != nil {
			return nil, errors.Join(fmt.Errorf("validate build staging ancestor %s: %w", current, err), security.Close(), closeBuildStagingAncestors(ancestors))
		}
		ancestors = append(ancestors, buildStagingAncestor{directory: current, info: info, security: security})
		if filepath.Dir(current) == current {
			return ancestors, nil
		}
	}
	return ancestors, nil
}

func validateBuildStagingAncestors(parentDirectory string, ancestors []buildStagingAncestor) error {
	expected := filepath.Dir(parentDirectory)
	if expected == parentDirectory {
		if len(ancestors) != 0 {
			return fmt.Errorf("retained build staging ancestor chain is invalid")
		}
		return nil
	}
	for index, ancestor := range ancestors {
		if ancestor.directory != expected || ancestor.info == nil || ancestor.security == nil {
			return fmt.Errorf("retained build staging ancestor chain is incomplete")
		}
		retained, retainedErr := ancestor.security.Stat()
		current, currentErr := os.Lstat(ancestor.directory)
		if retainedErr != nil || currentErr != nil || retained == nil || current == nil || !retained.IsDir() ||
			current.Mode()&os.ModeSymlink != 0 || !os.SameFile(ancestor.info, retained) || !os.SameFile(ancestor.info, current) {
			if retainedErr == nil && currentErr == nil {
				retainedErr = fmt.Errorf("build staging ancestor changed identity")
			}
			return fmt.Errorf("validate retained build staging ancestor %s: %w", ancestor.directory, errors.Join(retainedErr, currentErr))
		}
		if err := validateBuildStagingParentProtection(ancestor.security, retained); err != nil {
			return fmt.Errorf("validate retained build staging ancestor %s: %w", ancestor.directory, err)
		}
		next := filepath.Dir(expected)
		if next == expected {
			if index != len(ancestors)-1 {
				return fmt.Errorf("retained build staging ancestor chain is invalid")
			}
			return nil
		}
		expected = next
	}
	return fmt.Errorf("retained build staging ancestor chain is incomplete")
}

func closeBuildStagingAncestors(ancestors []buildStagingAncestor) error {
	var result error
	for index := len(ancestors) - 1; index >= 0; index-- {
		if ancestors[index].security != nil {
			result = errors.Join(result, ancestors[index].security.Close())
		}
	}
	return result
}

func requireBuildStagingOutsideProject(directory string, projectIdentity fs.FileInfo) error {
	canonical, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return fmt.Errorf("resolve build staging directory symlinks: %w", err)
	}
	current, err := filepath.Abs(canonical)
	if err != nil {
		return fmt.Errorf("resolve absolute build staging directory: %w", err)
	}
	for depth := 0; depth < 1024; depth++ {
		info, err := os.Stat(current)
		if err != nil {
			return fmt.Errorf("inspect build staging ancestry %s: %w", current, err)
		}
		if os.SameFile(projectIdentity, info) {
			return fmt.Errorf("build staging directory must be outside the project root")
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		current = parent
	}
	return fmt.Errorf("build staging ancestry exceeds the supported depth")
}

func removeBuildStagingFromParent(parent *os.Root, name string, identity fs.FileInfo) error {
	if parent == nil || name == "" || identity == nil {
		return fmt.Errorf("refuse to remove build staging directory without its identity")
	}
	current, err := parent.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect build staging directory for removal: %w", err)
	}
	if !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(identity, current) {
		return fmt.Errorf("refuse to remove build staging directory after its pathname changed")
	}
	if err := parent.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove build staging directory: %w", err)
	}
	return nil
}

// OutputPath returns the isolated compiler destination.
func (staging *buildStaging) OutputPath() string {
	if staging == nil {
		return ""
	}
	return filepath.Join(staging.directory, buildStagingOutputName)
}

// OpenOutput opens the compiler result through the retained staging Root.
func (staging *buildStaging) OpenOutput() (*os.File, error) {
	if err := staging.validatePathBinding(); err != nil {
		return nil, err
	}
	before, err := staging.root.Lstat(buildStagingOutputName)
	if err != nil {
		return nil, fmt.Errorf("inspect staged Go build output: %w", err)
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() || before.Size() == 0 {
		return nil, fmt.Errorf("Go build did not produce a non-empty regular output")
	}
	file, err := staging.root.Open(buildStagingOutputName)
	if err != nil {
		return nil, fmt.Errorf("open staged Go build output: %w", err)
	}
	opened, statErr := file.Stat()
	if statErr != nil || !os.SameFile(before, opened) || !opened.Mode().IsRegular() || opened.Size() == 0 {
		if statErr == nil {
			statErr = fmt.Errorf("staged Go build output changed while it was opened")
		}
		return nil, errors.Join(statErr, file.Close())
	}
	return file, nil
}

func (staging *buildStaging) validatePathBinding() error {
	if staging == nil || staging.parent == nil || staging.parentSecurity == nil || staging.root == nil || staging.info == nil || staging.parentInfo == nil {
		return fmt.Errorf("build staging directory is unavailable")
	}
	if err := staging.validateRetainedRoot(); err != nil {
		return err
	}
	currentParent, parentErr := os.Lstat(staging.parentDirectory)
	if parentErr != nil || currentParent == nil || !currentParent.IsDir() || currentParent.Mode()&os.ModeSymlink != 0 ||
		!os.SameFile(staging.parentInfo, currentParent) {
		if parentErr == nil {
			parentErr = fmt.Errorf("operating system temporary-directory pathname changed")
		}
		return fmt.Errorf("validate build staging parent directory: %w", parentErr)
	}
	current, err := os.Lstat(staging.directory)
	if err != nil || current == nil || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(staging.info, current) {
		if err == nil {
			err = fmt.Errorf("build staging pathname changed")
		}
		return fmt.Errorf("validate build staging directory: %w", err)
	}
	return requireBuildStagingOutsideProject(staging.directory, staging.projectIdentity)
}

func (staging *buildStaging) validateRetainedRoot() error {
	if staging == nil || staging.parent == nil || staging.parentSecurity == nil || staging.root == nil || staging.security == nil ||
		staging.parentInfo == nil || staging.info == nil {
		return fmt.Errorf("build staging directory is unavailable")
	}
	parentInfo, parentErr := staging.parent.Stat(".")
	parentSecurityInfo, parentSecurityErr := staging.parentSecurity.Stat()
	childInfo, childErr := staging.parent.Lstat(staging.name)
	opened, err := staging.root.Stat(".")
	securityInfo, securityErr := staging.security.Stat()
	if parentErr != nil || parentSecurityErr != nil || childErr != nil || err != nil || securityErr != nil || parentInfo == nil ||
		parentSecurityInfo == nil || childInfo == nil || opened == nil || securityInfo == nil || !parentInfo.IsDir() ||
		!parentSecurityInfo.IsDir() || !childInfo.IsDir() || !opened.IsDir() || !securityInfo.IsDir() ||
		!os.SameFile(staging.parentInfo, parentInfo) || !os.SameFile(staging.parentInfo, parentSecurityInfo) || !os.SameFile(staging.info, childInfo) ||
		!os.SameFile(staging.info, opened) || !os.SameFile(staging.info, securityInfo) {
		joined := errors.Join(parentErr, parentSecurityErr, childErr, err, securityErr)
		if joined == nil {
			joined = fmt.Errorf("retained build staging handles changed identity")
		}
		return fmt.Errorf("validate retained build staging handles: %w", joined)
	}
	if err := validateBuildStagingParentProtection(staging.parentSecurity, parentSecurityInfo); err != nil {
		return err
	}
	if err := validateBuildStagingAncestors(staging.parentDirectory, staging.ancestors); err != nil {
		return err
	}
	if err := validateBuildStagingProtection(staging.security, securityInfo); err != nil {
		return err
	}
	return nil
}

// Close removes the staged output and its private directory at most once.
func (staging *buildStaging) Close() error {
	if staging == nil {
		return nil
	}
	staging.once.Do(func() {
		bindingErr := staging.validatePathBinding()
		rootErr := staging.validateRetainedRoot()
		var removeOutputErr error
		if rootErr == nil {
			if err := staging.root.Remove(buildStagingOutputName); err != nil && !errors.Is(err, fs.ErrNotExist) {
				removeOutputErr = fmt.Errorf("remove staged Go build output: %w", err)
			}
		}
		var closeErr error
		if staging.root != nil {
			closeErr = staging.root.Close()
			staging.root = nil
		}
		var securityCloseErr error
		if staging.security != nil {
			securityCloseErr = staging.security.Close()
			staging.security = nil
		}
		removeDirectoryErr := removeBuildStagingFromParent(staging.parent, staging.name, staging.info)
		var parentSecurityCloseErr error
		if staging.parentSecurity != nil {
			parentSecurityCloseErr = staging.parentSecurity.Close()
			staging.parentSecurity = nil
		}
		ancestorCloseErr := closeBuildStagingAncestors(staging.ancestors)
		staging.ancestors = nil
		var parentCloseErr error
		if staging.parent != nil {
			parentCloseErr = staging.parent.Close()
			staging.parent = nil
		}
		staging.err = errors.Join(
			bindingErr,
			rootErr,
			removeOutputErr,
			closeErr,
			securityCloseErr,
			removeDirectoryErr,
			parentSecurityCloseErr,
			ancestorCloseErr,
			parentCloseErr,
		)
	})
	return staging.err
}

func copyStagedBuildOutput(ctx context.Context, root *os.Root, target string, source *os.File) (name string, result error) {
	if source == nil {
		return "", fmt.Errorf("staged Go build output is unavailable")
	}
	before, err := source.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() == 0 {
		if err == nil {
			err = fmt.Errorf("staged Go build output is invalid")
		}
		return "", errors.Join(err, source.Close())
	}
	name, destination, err := createSiblingTemporaryContext(ctx, root, target, ".build")
	if err != nil {
		return "", errors.Join(err, source.Close())
	}
	cleanupName := name
	keep := false
	defer func() {
		if !keep {
			if removeErr := root.Remove(cleanupName); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
				result = errors.Join(result, fmt.Errorf("remove incomplete build output: %w", removeErr))
			}
		}
	}()

	written, copyErr := copyBuildFileContext(ctx, destination, source)
	afterSource, sourceStatErr := source.Stat()
	sourceCloseErr := source.Close()
	chmodErr := destination.Chmod(0o755)
	syncErr := destination.Sync()
	destinationInfo, destinationStatErr := destination.Stat()
	destinationCloseErr := destination.Close()
	afterDestination, lstatErr := root.Lstat(name)
	if copyErr != nil || sourceStatErr != nil || sourceCloseErr != nil || chmodErr != nil || syncErr != nil ||
		destinationStatErr != nil || destinationCloseErr != nil || lstatErr != nil {
		return "", fmt.Errorf("copy staged Go build output: %w", errors.Join(
			copyErr, sourceStatErr, sourceCloseErr, chmodErr, syncErr,
			destinationStatErr, destinationCloseErr, lstatErr,
		))
	}
	if written != before.Size() || !os.SameFile(before, afterSource) || afterSource.Size() != before.Size() ||
		destinationInfo == nil || afterDestination == nil || !destinationInfo.Mode().IsRegular() ||
		!afterDestination.Mode().IsRegular() || !os.SameFile(destinationInfo, afterDestination) ||
		destinationInfo.Size() != before.Size() || !validBuildOutputMode(destinationInfo.Mode()) {
		return "", fmt.Errorf("staged Go build output changed while it was copied")
	}
	keep = true
	return name, nil
}

func copyBuildFileContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, 128*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		read, readErr := source.Read(buffer)
		if read > 0 {
			written, writeErr := destination.Write(buffer[:read])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != read {
				return total, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return total, nil
			}
			return total, readErr
		}
		if read == 0 {
			return total, io.ErrNoProgress
		}
	}
}

type buildTargetState struct {
	exists  bool
	info    fs.FileInfo
	mode    fs.FileMode
	size    int64
	modTime time.Time
}

func copyBuildTargetToSibling(ctx context.Context, root *os.Root, target string, state buildTargetState) (name string, result error) {
	name, destination, err := createSiblingTemporaryContext(ctx, root, target, ".bak")
	if err != nil {
		return "", fmt.Errorf("prepare build output backup: %w", err)
	}
	cleanupName := name
	keep := false
	defer func() {
		if !keep {
			if removeErr := root.Remove(cleanupName); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
				result = errors.Join(result, fmt.Errorf("remove incomplete build output backup: %w", removeErr))
			}
		}
	}()

	source, err := root.Open(target)
	if err != nil {
		return "", errors.Join(fmt.Errorf("open build output for backup: %w", err), destination.Close())
	}
	opened, statErr := source.Stat()
	if statErr != nil || !state.matchesInfo(opened) {
		if statErr == nil {
			statErr = fmt.Errorf("build output changed before backup")
		}
		return "", errors.Join(statErr, source.Close(), destination.Close())
	}
	written, copyErr := copyBuildFileContext(ctx, destination, source)
	afterRead, sourceStatErr := source.Stat()
	sourceCloseErr := source.Close()
	chmodErr := destination.Chmod(state.mode)
	syncErr := destination.Sync()
	backupInfo, backupStatErr := destination.Stat()
	destinationCloseErr := destination.Close()
	if copyErr != nil || sourceStatErr != nil || sourceCloseErr != nil || chmodErr != nil || syncErr != nil || backupStatErr != nil || destinationCloseErr != nil {
		return "", fmt.Errorf("copy build output backup: %w", errors.Join(
			copyErr, sourceStatErr, sourceCloseErr, chmodErr, syncErr, backupStatErr, destinationCloseErr,
		))
	}
	if written != state.size || !state.matchesInfo(afterRead) || backupInfo == nil ||
		!backupInfo.Mode().IsRegular() || backupInfo.Size() != state.size || backupInfo.Mode() != state.mode {
		return "", fmt.Errorf("build output changed while it was backed up")
	}
	if err := state.matches(root, target); err != nil {
		return "", err
	}
	keep = true
	return name, nil
}

func removeUnusedBuildBackup(root *os.Root, name string) error {
	if name == "" {
		return nil
	}
	if err := root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove unused build backup: %w", err)
	}
	return nil
}

func (state buildTargetState) matchesInfo(info fs.FileInfo) bool {
	return state.exists && state.info != nil && info != nil && info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular() &&
		os.SameFile(state.info, info) && info.Mode() == state.mode && info.Size() == state.size && info.ModTime().Equal(state.modTime)
}

func inspectBuildTarget(root *os.Root, target string) (buildTargetState, error) {
	info, err := root.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return buildTargetState{}, nil
	}
	if err != nil {
		return buildTargetState{}, fmt.Errorf("inspect build output %s: %w", target, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return buildTargetState{}, fmt.Errorf("build output %s must be a regular non-symlink file", target)
	}
	return buildTargetState{exists: true, info: info, mode: info.Mode(), size: info.Size(), modTime: info.ModTime()}, nil
}

func (state buildTargetState) matches(root *os.Root, target string) error {
	current, err := root.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) && !state.exists {
		return nil
	}
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("build output %s changed concurrently", target)
		}
		return fmt.Errorf("inspect build output %s: %w", target, err)
	}
	if !state.matchesInfo(current) {
		return fmt.Errorf("build output %s changed concurrently", target)
	}
	return nil
}

type guardedWriter struct {
	gate      *sync.Mutex
	target    io.Writer
	operation string
	err       error
}

func newBuildWriters(options BuildOptions) (*guardedWriter, *guardedWriter, error) {
	if isTypedNil(options.Stdout) || isTypedNil(options.Stderr) {
		return nil, nil, newUsageError("build output writers cannot be typed nil")
	}
	stdout := options.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := options.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	gate := &sync.Mutex{}
	return &guardedWriter{gate: gate, target: stdout, operation: "build stdout"},
		&guardedWriter{gate: gate, target: stderr, operation: "build stderr"}, nil
}

// Write serializes compiler output, contains writer panics, and retains the
// first output error for Build to join with command failures.
func (writer *guardedWriter) Write(data []byte) (written int, result error) {
	writer.gate.Lock()
	defer writer.gate.Unlock()
	if writer.err != nil {
		return 0, writer.err
	}
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			result = &CallbackPanicError{Operation: writer.operation}
			writer.err = result
			written = 0
		}
	}()
	written, result = writer.target.Write(data)
	if written < 0 || written > len(data) {
		countErr := fmt.Errorf("%s returned invalid byte count %d", writer.operation, written)
		result = errors.Join(countErr, result)
		written = 0
	} else if written != len(data) && result == nil {
		result = io.ErrShortWrite
	}
	if result != nil {
		if _, internal := result.(*CallbackPanicError); !internal {
			result = &buildWriterError{operation: writer.operation, cause: result}
		}
		writer.err = result
	}
	returned = true
	return written, result
}

// Err returns the first compiler-output error observed by Write.
func (writer *guardedWriter) Err() error {
	writer.gate.Lock()
	defer writer.gate.Unlock()
	return writer.err
}

func isTypedNil(value any) bool {
	if value == nil {
		return false
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
