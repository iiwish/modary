package projecttool

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/iiwish/modary/appkit"
	"gopkg.in/yaml.v3"
)

const (
	// ProjectManifestName is the fixed consumer project manifest name.
	ProjectManifestName = "modary.yaml"
	// MaximumManifestBytes bounds configuration input before YAML decoding.
	MaximumManifestBytes     = int64(1 << 20)
	maximumConfiguredPathLen = 1024
	maximumPathComponentLen  = 180
	portablePathGrammar      = "[A-Za-z0-9._-]+"
	maximumYAMLNodes         = 4096
	maximumYAMLDepth         = 32
)

// Manifest is the strict consumer project file. Go Definition composition is
// deliberately absent: application code remains the sole Module source.
type Manifest struct {
	Application appkit.Metadata `yaml:"application" json:"application"`
	Outputs     Outputs         `yaml:"outputs" json:"outputs"`
	Build       BuildTarget     `yaml:"build" json:"build"`
}

// Outputs declares consumer-owned generated artifact locations. Paths use `/`
// separators and portable ASCII components matching [A-Za-z0-9._-]+.
type Outputs struct {
	Graph      string `yaml:"graph" json:"graph"`
	Actions    string `yaml:"actions" json:"actions"`
	TypeScript string `yaml:"typescript,omitempty" json:"typescript,omitempty"`
}

// BuildTarget declares the consumer Go package and final binary location using
// the same portable path grammar as Outputs.
type BuildTarget struct {
	Package string `yaml:"package" json:"package"`
	Output  string `yaml:"output" json:"output"`
}

// Project is an immutable validated project-root and output policy. It stores
// no Module composition and owns no open resource.
type Project struct {
	root         string
	rootIdentity fs.FileInfo
	manifestHash [sha256.Size]byte
	manifest     Manifest
}

// ParseManifest parses exactly one strict YAML document. YAML aliases and
// anchors are rejected so configuration meaning is local and reviewable.
func ParseManifest(data []byte) (Manifest, error) {
	if len(data) == 0 {
		return Manifest{}, fmt.Errorf("project manifest is empty")
	}
	if int64(len(data)) > MaximumManifestBytes {
		return Manifest{}, fmt.Errorf("project manifest exceeds %d bytes", MaximumManifestBytes)
	}
	if !utf8.Valid(data) {
		return Manifest{}, fmt.Errorf("project manifest must be valid UTF-8")
	}

	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&document); err != nil {
		return Manifest{}, fmt.Errorf("parse project manifest: %w", err)
	}
	if err := requireYAMLEOF(decoder); err != nil {
		return Manifest{}, err
	}
	if err := rejectYAMLAliases(&document); err != nil {
		return Manifest{}, err
	}

	var manifest Manifest
	strict := yaml.NewDecoder(bytes.NewReader(data))
	strict.KnownFields(true)
	if err := strict.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse project manifest: %w", err)
	}
	if err := requireYAMLEOF(strict); err != nil {
		return Manifest{}, err
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func requireYAMLEOF(decoder *yaml.Decoder) error {
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("project manifest must contain exactly one YAML document")
		}
		return fmt.Errorf("parse project manifest: %w", err)
	}
	return nil
}

func rejectYAMLAliases(value *yaml.Node) error {
	if value == nil {
		return nil
	}
	type pendingNode struct {
		value *yaml.Node
		depth int
	}
	pending := []pendingNode{{value: value, depth: 0}}
	visited := 0
	for len(pending) != 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		visited++
		if visited > maximumYAMLNodes || current.depth > maximumYAMLDepth {
			return fmt.Errorf("project manifest YAML structure is too complex")
		}
		if current.value.Kind == yaml.AliasNode || current.value.Alias != nil || current.value.Anchor != "" {
			return fmt.Errorf("project manifest aliases and anchors are not allowed")
		}
		for _, child := range current.value.Content {
			pending = append(pending, pendingNode{value: child, depth: current.depth + 1})
		}
	}
	return nil
}

func validateManifest(manifest Manifest) error {
	if err := appkit.ValidateMetadata(manifest.Application); err != nil {
		return fmt.Errorf("project application: %w", err)
	}
	paths := []struct {
		name     string
		value    string
		required bool
	}{
		{"outputs.graph", manifest.Outputs.Graph, true},
		{"outputs.actions", manifest.Outputs.Actions, true},
		{"outputs.typescript", manifest.Outputs.TypeScript, false},
		{"build.output", manifest.Build.Output, true},
	}
	for _, configured := range paths {
		if err := validateRelativePath(configured.name, configured.value, configured.required); err != nil {
			return err
		}
	}
	if err := validateBuildPackage(manifest.Build.Package); err != nil {
		return err
	}
	outputs := configuredOutputs(manifest)
	packagePath := strings.TrimPrefix(manifest.Build.Package, "./")
	for index, first := range outputs {
		if aliasesPath(first, ProjectManifestName) {
			return fmt.Errorf("configured output %q aliases %s", first, ProjectManifestName)
		}
		if aliasesPath(first, packagePath) {
			return fmt.Errorf("configured output %q and build.package %q alias or contain one another", first, manifest.Build.Package)
		}
		for _, second := range outputs[index+1:] {
			if aliasesPath(first, second) {
				return fmt.Errorf("configured outputs %q and %q alias or contain one another", first, second)
			}
		}
	}
	if aliasesPath(packagePath, ProjectManifestName) {
		return fmt.Errorf("build.package %q aliases or contains %s", manifest.Build.Package, ProjectManifestName)
	}
	return nil
}

func validateRelativePath(field, value string, required bool) error {
	if value == "" && !required {
		return nil
	}
	if value == "" || len(value) > maximumConfiguredPathLen || !utf8.ValidString(value) ||
		strings.TrimSpace(value) != value || strings.Contains(value, "\\") ||
		strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("project %s path is invalid", field)
	}
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || filepath.IsAbs(value) || filepath.VolumeName(value) != "" || looksLikeWindowsVolume(value) {
		return fmt.Errorf("project %s path must be relative to the consumer root", field)
	}
	if value == "." || path.Clean(value) != value {
		return fmt.Errorf("project %s path must be canonical", field)
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" || component == "." || component == ".." || len(component) > maximumPathComponentLen ||
			!isPortablePathComponent(component) || strings.HasSuffix(component, ".") ||
			isWindowsDeviceName(component) {
			return fmt.Errorf("project %s path component must match portable grammar %s", field, portablePathGrammar)
		}
	}
	return nil
}

func isPortablePathComponent(component string) bool {
	for index := 0; index < len(component); index++ {
		value := component[index]
		if (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') ||
			(value >= '0' && value <= '9') || value == '.' || value == '_' || value == '-' {
			continue
		}
		return false
	}
	return component != ""
}

func looksLikeWindowsVolume(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':'
}

func isWindowsDeviceName(component string) bool {
	base := component
	if index := strings.IndexByte(base, '.'); index >= 0 {
		base = base[:index]
	}
	base = strings.ToUpper(base)
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" {
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9'
}

func validateBuildPackage(value string) error {
	if !strings.HasPrefix(value, "./") || value == "./" {
		return fmt.Errorf("project build.package must be a non-root relative Go package")
	}
	packagePath := strings.TrimPrefix(value, "./")
	if err := validateRelativePath("build.package", packagePath, true); err != nil {
		return err
	}
	for _, component := range strings.Split(packagePath, "/") {
		if component == "..." {
			return fmt.Errorf("project build.package must identify one Go package")
		}
	}
	return nil
}

func configuredOutputs(manifest Manifest) []string {
	result := []string{manifest.Outputs.Graph, manifest.Outputs.Actions, manifest.Build.Output}
	if manifest.Outputs.TypeScript != "" {
		result = append(result, manifest.Outputs.TypeScript)
	}
	return result
}

func aliasesPath(first, second string) bool {
	firstParts := strings.Split(first, "/")
	secondParts := strings.Split(second, "/")
	shared := len(firstParts)
	if len(secondParts) < shared {
		shared = len(secondParts)
	}
	for index := 0; index < shared; index++ {
		if !strings.EqualFold(firstParts[index], secondParts[index]) {
			return false
		}
	}
	return true
}

// Load validates the canonical root, its strict modary.yaml, output aliases,
// and every existing output path component without following symlinks.
func Load(root string) (*Project, error) {
	return LoadContext(context.Background(), root)
}

// LoadContext is the cancelable form of Load. The root directory is opened
// before its identity is inspected, and that verified handle is used for the
// entire load operation.
func LoadContext(ctx context.Context, root string) (*Project, error) {
	if ctx == nil {
		return nil, ErrContextRequired
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	canonicalRoot, err := canonicalProjectRoot(root)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rootHandle, err := os.OpenRoot(canonicalRoot)
	if err != nil {
		return nil, fmt.Errorf("open project root: %w", err)
	}
	closeRoot := true
	defer func() {
		if closeRoot {
			_ = rootHandle.Close()
		}
	}()
	openedRoot, err := rootHandle.Stat(".")
	if err != nil {
		return nil, fmt.Errorf("inspect opened project root: %w", err)
	}
	if !openedRoot.IsDir() {
		return nil, fmt.Errorf("project root is not a directory")
	}
	currentRoot, err := os.Stat(canonicalRoot)
	if err != nil {
		return nil, fmt.Errorf("inspect project root pathname: %w", err)
	}
	if !os.SameFile(openedRoot, currentRoot) {
		return nil, fmt.Errorf("project root changed while it was loaded")
	}
	data, err := readRootRegularFileContext(ctx, rootHandle, ProjectManifestName, MaximumManifestBytes)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	manifest, err := ParseManifest(data)
	if err != nil {
		return nil, err
	}
	project := &Project{
		root:         canonicalRoot,
		rootIdentity: openedRoot,
		manifestHash: sha256.Sum256(data),
		manifest:     manifest,
	}
	if err := project.validateFilesystemPathsContext(ctx, rootHandle); err != nil {
		return nil, err
	}
	if err := project.verifyRootPathBinding(rootHandle); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	closeRoot = false
	if err := rootHandle.Close(); err != nil {
		return nil, fmt.Errorf("close project root: %w", err)
	}
	return project, nil
}

func canonicalProjectRoot(root string) (string, error) {
	if root == "" {
		root = "."
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve project root symlinks: %w", err)
	}
	return filepath.Clean(canonical), nil
}

func readRootRegularFile(root *os.Root, name string, limit int64) ([]byte, error) {
	return readRootRegularFileContext(context.Background(), root, name, limit)
}

func readRootRegularFileContext(ctx context.Context, root *os.Root, name string, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	before, err := root.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", name, err)
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular non-symlink file", name)
	}
	if before.Size() > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", name, limit)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	openedBefore, statErr := file.Stat()
	data, readErr := readAllContext(ctx, &io.LimitedReader{R: file, N: limit + 1})
	openedAfter, restatErr := file.Stat()
	closeErr := file.Close()
	after, lstatErr := root.Lstat(name)
	if statErr != nil || readErr != nil || restatErr != nil || closeErr != nil || lstatErr != nil {
		return nil, fmt.Errorf("read %s consistently: %w", name, errors.Join(statErr, readErr, restatErr, closeErr, lstatErr))
	}
	if after.Mode()&os.ModeSymlink != 0 || !after.Mode().IsRegular() ||
		!sameFileState(before, openedBefore) || !sameFileState(openedBefore, openedAfter) || !sameFileState(openedAfter, after) {
		return nil, fmt.Errorf("%s changed while it was read", name)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", name, limit)
	}
	return data, nil
}

func readAllContext(ctx context.Context, reader io.Reader) ([]byte, error) {
	var data bytes.Buffer
	buffer := make([]byte, 32<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		read, err := reader.Read(buffer)
		if read > 0 {
			_, _ = data.Write(buffer[:read])
		}
		if err == io.EOF {
			return data.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
		if read == 0 {
			return nil, io.ErrNoProgress
		}
	}
}

func sameFileState(first, second fs.FileInfo) bool {
	return first != nil && second != nil && os.SameFile(first, second) &&
		first.Mode() == second.Mode() && first.Size() == second.Size() && first.ModTime().Equal(second.ModTime())
}

func (project *Project) openVerifiedRoot(ctx context.Context) (*os.Root, error) {
	if ctx == nil {
		return nil, ErrContextRequired
	}
	if project == nil {
		return nil, fmt.Errorf("project is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(project.root)
	if err != nil {
		return nil, fmt.Errorf("open project root: %w", err)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = root.Close()
		}
	}()
	if err := project.verifyRootPathBinding(root); err != nil {
		return nil, err
	}
	if err := project.validateFilesystemPathsContext(ctx, root); err != nil {
		return nil, err
	}
	if err := project.verifyRootPathBinding(root); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	closeOnError = false
	return root, nil
}

func (project *Project) verifyRootPathBinding(root *os.Root) error {
	if project == nil || project.rootIdentity == nil || root == nil {
		return fmt.Errorf("project is required")
	}
	openedRoot, err := root.Stat(".")
	if err != nil {
		return fmt.Errorf("inspect opened project root: %w", err)
	}
	if !openedRoot.IsDir() || !os.SameFile(project.rootIdentity, openedRoot) {
		return fmt.Errorf("project root handle changed after it was loaded")
	}
	currentRoot, err := os.Stat(project.root)
	if err != nil {
		return fmt.Errorf("inspect project root pathname: %w", err)
	}
	if !currentRoot.IsDir() || !os.SameFile(openedRoot, currentRoot) {
		return fmt.Errorf("project root pathname changed after it was loaded")
	}
	return nil
}

func (project *Project) validateFilesystemPathsContext(ctx context.Context, root *os.Root) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	manifestData, err := readRootRegularFileContext(ctx, root, ProjectManifestName, MaximumManifestBytes)
	if err != nil {
		return err
	}
	if sha256.Sum256(manifestData) != project.manifestHash {
		return fmt.Errorf("%s changed after the project was loaded", ProjectManifestName)
	}
	manifestInfo, err := root.Lstat(ProjectManifestName)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", ProjectManifestName, err)
	}
	if manifestInfo.Mode()&os.ModeSymlink != 0 || !manifestInfo.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular non-symlink file", ProjectManifestName)
	}
	paths := configuredOutputs(project.manifest)
	infos := make(map[string]fs.FileInfo, len(paths))
	for _, name := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := validateExistingOutputPathContext(ctx, root, name)
		if err != nil {
			return err
		}
		if info != nil {
			if os.SameFile(manifestInfo, info) {
				return fmt.Errorf("configured output %q refers to %s", name, ProjectManifestName)
			}
			infos[name] = info
		}
	}
	for index, first := range paths {
		firstInfo := infos[first]
		if firstInfo == nil {
			continue
		}
		for _, second := range paths[index+1:] {
			if secondInfo := infos[second]; secondInfo != nil && os.SameFile(firstInfo, secondInfo) {
				return fmt.Errorf("configured outputs %q and %q refer to the same file", first, second)
			}
		}
	}
	if err := validateConfiguredDirectoryPathContext(ctx, root, strings.TrimPrefix(project.manifest.Build.Package, "./"), false); err != nil {
		return fmt.Errorf("validate build package: %w", err)
	}
	return ctx.Err()
}

func validateExistingOutputPath(root *os.Root, name string) (fs.FileInfo, error) {
	return validateExistingOutputPathContext(context.Background(), root, name)
}

func validateExistingOutputPathContext(ctx context.Context, root *os.Root, name string) (fs.FileInfo, error) {
	components := strings.Split(name, "/")
	for index := range components {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		current := strings.Join(components[:index+1], "/")
		info, err := root.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("inspect configured path %s: %w", name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("configured path %s traverses symbolic link %s", name, current)
		}
		if index < len(components)-1 && !info.IsDir() {
			return nil, fmt.Errorf("configured path %s has non-directory component %s", name, current)
		}
		if index == len(components)-1 {
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("configured output %s is not a regular file", name)
			}
			return info, nil
		}
	}
	return nil, nil
}

func validateConfiguredDirectoryPath(root *os.Root, name string, required bool) error {
	return validateConfiguredDirectoryPathContext(context.Background(), root, name, required)
}

func validateConfiguredDirectoryPathContext(ctx context.Context, root *os.Root, name string, required bool) error {
	components := strings.Split(name, "/")
	for index := range components {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := strings.Join(components[:index+1], "/")
		info, err := root.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			if required {
				return fmt.Errorf("configured directory %s does not exist", name)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect configured directory %s: %w", name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("configured directory %s traverses non-directory or symbolic link %s", name, current)
		}
	}
	return nil
}

// Root returns the canonical consumer root captured by Load.
func (project *Project) Root() string {
	if project == nil {
		return ""
	}
	return project.root
}

// Manifest returns the validated, output-only consumer manifest.
func (project *Project) Manifest() Manifest {
	if project == nil {
		return Manifest{}
	}
	return project.manifest
}
