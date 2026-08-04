package starter

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

const (
	// DefaultModaryVersion is written by the current Starter when a caller does
	// not select another exact framework version.
	DefaultModaryVersion = "v0.3.0-alpha.1"
)

var (
	// ErrContextRequired reports a nil creation or command context.
	ErrContextRequired = errors.New("starter context is required")
	// ErrInvalidOptions reports invalid project identity, module, or Profile input.
	ErrInvalidOptions = errors.New("starter options are invalid")
	// ErrDestinationNotEmpty reports that create-only ownership cannot be established.
	ErrDestinationNotEmpty = errors.New("starter destination is not empty")
	// ErrUnsafeDestination reports a symlink or non-directory destination.
	ErrUnsafeDestination = errors.New("starter destination is unsafe")
)

var projectIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// Profile identifies a visible source preset used only during project creation.
type Profile string

const (
	// ProfileAPI creates the database-free standard Go HTTP application.
	ProfileAPI Profile = "api"
	// ProfileAdmin creates the optional administrative application.
	ProfileAdmin Profile = "admin"
	// ProfileGoverned creates the optional governed-operation application.
	ProfileGoverned Profile = "governed"
)

// Component identifies one optional Profile-owned component selection.
type Component string

const (
	// ComponentTasks adds River-backed task inspection to the Admin Profile.
	ComponentTasks Component = "tasks"
	// ComponentAudit adds scope-bound audit inspection to the Admin Profile.
	ComponentAudit Component = "audit"
	// ComponentOIDC replaces the generated local-password login surface with
	// the official OIDC component and redirect contribution.
	ComponentOIDC Component = "oidc"
	// ComponentOTel adds the independently versioned OTLP traces and metrics
	// adapter without changing the generated Admin product surface.
	ComponentOTel Component = "otel"
)

// CreateOptions defines one initial project rendering. ModaryReplace is an
// explicit development and conformance hook; released projects normally leave
// it empty and use the exact ModaryVersion.
type CreateOptions struct {
	Destination   string
	ModulePath    string
	Name          string
	Profile       Profile
	ModaryVersion string
	ModaryReplace string
	Components    []Component
}

// Result identifies the created project and its sorted consumer-owned files.
type Result struct {
	Destination string      `json:"destination"`
	Profile     Profile     `json:"profile"`
	Files       []string    `json:"files"`
	Components  []Component `json:"components,omitempty"`
}

type normalizedCreateOptions struct {
	destination   string
	parent        string
	base          string
	id            string
	modulePath    string
	name          string
	profile       Profile
	modaryVersion string
	modaryReplace string
	components    []Component
}

type renderedFile struct {
	path string
	data []byte
}

func (options normalizedCreateOptions) hasComponent(component Component) bool {
	index := sort.Search(len(options.components), func(index int) bool { return options.components[index] >= component })
	return index < len(options.components) && options.components[index] == component
}

// Create renders and writes a complete Profile only after all input and
// templates validate. It accepts a nonexistent or empty real directory and
// never merges with, replaces, or patches an existing file.
func Create(ctx context.Context, options CreateOptions) (Result, error) {
	if ctx == nil {
		return Result{}, ErrContextRequired
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	normalized, err := normalizeCreateOptions(options)
	if err != nil {
		return Result{}, err
	}
	files, err := renderProfile(ctx, normalized)
	if err != nil {
		return Result{}, err
	}
	if err := writeProject(ctx, normalized, files); err != nil {
		return Result{}, err
	}
	paths := make([]string, len(files))
	for index, file := range files {
		paths[index] = file.path
	}
	return Result{Destination: normalized.destination, Profile: normalized.profile, Files: paths, Components: append([]Component(nil), normalized.components...)}, nil
}

func normalizeCreateOptions(options CreateOptions) (normalizedCreateOptions, error) {
	destination := options.Destination
	if destination == "" || strings.TrimSpace(destination) != destination || strings.ContainsRune(destination, '\x00') {
		return normalizedCreateOptions{}, fmt.Errorf("%w: destination is required", ErrInvalidOptions)
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return normalizedCreateOptions{}, fmt.Errorf("%w: resolve destination: %v", ErrInvalidOptions, err)
	}
	absolute = filepath.Clean(absolute)
	base := filepath.Base(absolute)
	if absolute == filepath.Dir(absolute) || !projectIDPattern.MatchString(base) {
		return normalizedCreateOptions{}, fmt.Errorf("%w: destination base %q must be a lowercase project id", ErrInvalidOptions, base)
	}
	parent := filepath.Dir(absolute)
	parentInfo, err := os.Stat(parent)
	if err != nil || !parentInfo.IsDir() {
		return normalizedCreateOptions{}, fmt.Errorf("%w: destination parent must be an existing directory", ErrInvalidOptions)
	}

	modulePath := options.ModulePath
	if modulePath == "" {
		modulePath = base
	}
	if err := validateModulePath(modulePath); err != nil {
		return normalizedCreateOptions{}, fmt.Errorf("%w: module path %q: %v", ErrInvalidOptions, modulePath, err)
	}
	name := options.Name
	if name == "" {
		name = displayName(base)
	}
	if err := validateProjectName(name); err != nil {
		return normalizedCreateOptions{}, err
	}
	profile := options.Profile
	if profile == "" {
		profile = ProfileAPI
	}
	switch profile {
	case ProfileAPI, ProfileAdmin, ProfileGoverned:
	default:
		return normalizedCreateOptions{}, fmt.Errorf("%w: profile %q is unknown", ErrInvalidOptions, profile)
	}
	components := append([]Component(nil), options.Components...)
	sort.Slice(components, func(left, right int) bool { return components[left] < components[right] })
	for index, component := range components {
		if component != ComponentTasks && component != ComponentAudit && component != ComponentOIDC && component != ComponentOTel {
			return normalizedCreateOptions{}, fmt.Errorf("%w: component %q is unknown", ErrInvalidOptions, component)
		}
		if profile != ProfileAdmin {
			return normalizedCreateOptions{}, fmt.Errorf("%w: component %q requires the Admin Profile", ErrInvalidOptions, component)
		}
		if index > 0 && component == components[index-1] {
			return normalizedCreateOptions{}, fmt.Errorf("%w: component %q was selected more than once", ErrInvalidOptions, component)
		}
	}
	version := options.ModaryVersion
	if version == "" {
		version = DefaultModaryVersion
	}
	if !semver.IsValid(version) || version == "v0.0.0" || strings.Contains(version, "+") {
		return normalizedCreateOptions{}, fmt.Errorf("%w: Modary version %q must be an exact semantic version", ErrInvalidOptions, version)
	}
	replace := options.ModaryReplace
	if replace != "" {
		replace, err = filepath.Abs(replace)
		if err != nil {
			return normalizedCreateOptions{}, fmt.Errorf("%w: resolve Modary replacement: %v", ErrInvalidOptions, err)
		}
		replace = filepath.Clean(replace)
		info, statErr := os.Stat(replace)
		if statErr != nil || !info.IsDir() {
			return normalizedCreateOptions{}, fmt.Errorf("%w: Modary replacement must be an existing directory", ErrInvalidOptions)
		}
	}
	return normalizedCreateOptions{
		destination: absolute, parent: parent, base: base, id: base,
		modulePath: modulePath, name: name, profile: profile,
		modaryVersion: version, modaryReplace: replace,
		components: components,
	}, nil
}

func validateModulePath(value string) error {
	if err := module.CheckPath(value); err != nil {
		return err
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "vendor" {
			return fmt.Errorf("path segment %q is reserved by Go vendoring", segment)
		}
	}
	return nil
}

func displayName(id string) string {
	parts := strings.Split(id, "-")
	for index, part := range parts {
		if part != "" {
			parts[index] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}

func validateProjectName(value string) error {
	if value == "" || !utf8.ValidString(value) || strings.TrimSpace(value) != value ||
		utf8.RuneCountInString(value) > 160 || strings.ContainsFunc(value, unicode.IsControl) {
		return fmt.Errorf("%w: project name is invalid", ErrInvalidOptions)
	}
	return nil
}

func writeProject(ctx context.Context, options normalizedCreateOptions, files []renderedFile) (resultErr error) {
	parent, err := os.OpenRoot(options.parent)
	if err != nil {
		return fmt.Errorf("open destination parent: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, parent.Close()) }()

	createdDestination := false
	info, err := parent.Lstat(options.base)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := parent.Mkdir(options.base, 0o755); err != nil {
			return fmt.Errorf("create destination: %w", err)
		}
		createdDestination = true
		info, err = parent.Lstat(options.base)
		if err != nil {
			return fmt.Errorf("inspect created destination: %w", err)
		}
	case err != nil:
		return fmt.Errorf("inspect destination: %w", err)
	case info.Mode()&os.ModeSymlink != 0 || !info.IsDir():
		return fmt.Errorf("%w: %s", ErrUnsafeDestination, options.destination)
	}

	root, err := parent.OpenRoot(options.base)
	if err != nil {
		if createdDestination && destinationMatches(parent, options.base, info) {
			_ = parent.Remove(options.base)
		}
		return fmt.Errorf("open destination: %w", err)
	}
	rootInfo, err := root.Stat(".")
	if err != nil || !os.SameFile(info, rootInfo) {
		_ = root.Close()
		if createdDestination && destinationMatches(parent, options.base, info) {
			_ = parent.Remove(options.base)
		}
		return fmt.Errorf("%w: destination changed while opening", ErrUnsafeDestination)
	}
	defer func() { resultErr = errors.Join(resultErr, root.Close()) }()

	createdFiles := make([]string, 0, len(files))
	createdDirectories := make([]string, 0)
	committed := false
	defer func() {
		if committed {
			return
		}
		for index := len(createdFiles) - 1; index >= 0; index-- {
			_ = root.Remove(createdFiles[index])
		}
		for index := len(createdDirectories) - 1; index >= 0; index-- {
			_ = root.Remove(createdDirectories[index])
		}
		if createdDestination && destinationMatches(parent, options.base, rootInfo) {
			_ = parent.Remove(options.base)
		}
	}()

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return fmt.Errorf("read destination: %w", err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("%w: %s", ErrDestinationNotEmpty, options.destination)
	}

	directories := profileDirectories(files)
	for _, directory := range directories {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := root.Mkdir(directory, 0o755); err != nil {
			return fmt.Errorf("create project directory %s: %w", directory, err)
		}
		createdDirectories = append(createdDirectories, directory)
	}
	for _, rendered := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		file, err := root.OpenFile(rendered.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return fmt.Errorf("create project file %s: %w", rendered.path, err)
		}
		createdFiles = append(createdFiles, rendered.path)
		if _, err := file.Write(rendered.data); err != nil {
			_ = file.Close()
			return fmt.Errorf("write project file %s: %w", rendered.path, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close project file %s: %w", rendered.path, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := verifyCreatedTree(root, files, directories); err != nil {
		return err
	}
	if !destinationMatches(parent, options.base, rootInfo) {
		return fmt.Errorf("%w: destination changed during creation", ErrUnsafeDestination)
	}
	committed = true
	return nil
}

func destinationMatches(parent *os.Root, base string, identity fs.FileInfo) bool {
	current, err := parent.Lstat(base)
	return err == nil && current.Mode()&os.ModeSymlink == 0 && current.IsDir() && os.SameFile(current, identity)
}

func verifyCreatedTree(root *os.Root, files []renderedFile, directories []string) error {
	expected := make(map[string]struct{}, len(files)+len(directories))
	for _, file := range files {
		expected[file.path] = struct{}{}
	}
	for _, directory := range directories {
		expected[directory] = struct{}{}
	}
	seen := 0
	err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: destination contains symlink %s", ErrUnsafeDestination, name)
		}
		if _, exists := expected[name]; !exists {
			return fmt.Errorf("%w: destination changed during creation", ErrDestinationNotEmpty)
		}
		seen++
		return nil
	})
	if err != nil {
		return err
	}
	if seen != len(expected) {
		return fmt.Errorf("verify created project: expected %d paths, found %d", len(expected), seen)
	}
	return nil
}

func profileDirectories(files []renderedFile) []string {
	set := make(map[string]struct{})
	for _, file := range files {
		for directory := filepath.ToSlash(filepath.Dir(file.path)); directory != "."; directory = filepath.ToSlash(filepath.Dir(directory)) {
			set[directory] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for directory := range set {
		result = append(result, directory)
	}
	sort.Slice(result, func(first, second int) bool {
		firstDepth := strings.Count(result[first], "/")
		secondDepth := strings.Count(result[second], "/")
		if firstDepth != secondDepth {
			return firstDepth < secondDepth
		}
		return result[first] < result[second]
	})
	return result
}
