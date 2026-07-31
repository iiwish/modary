package quality

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const modaryImportPath = "github.com/iiwish/modary"

type architectureLayer string

const (
	layerUnknown     architectureLayer = ""
	layerKernel      architectureLayer = "public kernel"
	layerAppKit      architectureLayer = "appkit"
	layerTransport   architectureLayer = "transport"
	layerAdapter     architectureLayer = "adapter"
	layerProjectTool architectureLayer = "projecttool"
	layerCommand     architectureLayer = "appcmd"
)

var allowedDirectLayerImports = map[architectureLayer]map[architectureLayer]bool{
	layerKernel:      {layerKernel: true},
	layerAppKit:      {layerKernel: true},
	layerTransport:   {layerKernel: true, layerAppKit: true, layerTransport: true},
	layerAdapter:     {layerKernel: true, layerAdapter: true},
	layerProjectTool: {layerKernel: true, layerAppKit: true},
	layerCommand:     {layerKernel: true, layerAppKit: true},
}

var publicProductionPackages = map[string]architectureLayer{
	modaryImportPath + "/action":                 layerKernel,
	modaryImportPath + "/audit":                  layerKernel,
	modaryImportPath + "/authz":                  layerKernel,
	modaryImportPath + "/database":               layerKernel,
	modaryImportPath + "/identity":               layerKernel,
	modaryImportPath + "/module":                 layerKernel,
	modaryImportPath + "/scope":                  layerKernel,
	modaryImportPath + "/appkit":                 layerAppKit,
	modaryImportPath + "/transport/httpapi":      layerTransport,
	modaryImportPath + "/adapters":               layerAdapter,
	modaryImportPath + "/adapters/localidentity": layerAdapter,
	modaryImportPath + "/adapters/rbac":          layerAdapter,
	modaryImportPath + "/adapters/sqlaudit":      layerAdapter,
	modaryImportPath + "/adapters/sqlite":        layerAdapter,
	modaryImportPath + "/projecttool":            layerProjectTool,
	modaryImportPath + "/appcmd":                 layerCommand,
}

var privilegedInternalImporters = map[string]map[string]bool{
	modaryImportPath + "/internal/actionpersistence": {
		modaryImportPath + "/adapters/sqlite": true,
	},
	modaryImportPath + "/internal/actionruntime": {
		modaryImportPath + "/module": true,
	},
	modaryImportPath + "/internal/databasecontrol": {
		modaryImportPath + "/module":                 true,
		modaryImportPath + "/adapters/localidentity": true,
		modaryImportPath + "/adapters/rbac":          true,
		modaryImportPath + "/adapters/sqlaudit":      true,
		modaryImportPath + "/adapters/sqlite":        true,
	},
	modaryImportPath + "/internal/filepolicy": {
		modaryImportPath + "/appcmd":          true,
		modaryImportPath + "/projecttool":     true,
		modaryImportPath + "/adapters/sqlite": true,
	},
	modaryImportPath + "/internal/jsonschema": {
		modaryImportPath + "/action":            true,
		modaryImportPath + "/transport/httpapi": true,
	},
	modaryImportPath + "/internal/jsonvalue": {
		modaryImportPath + "/action":            true,
		modaryImportPath + "/transport/httpapi": true,
	},
	modaryImportPath + "/internal/moduleassembly": {
		modaryImportPath + "/adapters/localidentity": true,
		modaryImportPath + "/adapters/rbac":          true,
		modaryImportPath + "/adapters/sqlaudit":      true,
		modaryImportPath + "/adapters/sqlite":        true,
	},
	modaryImportPath + "/internal/runtimecontrol": {
		modaryImportPath + "/module": true,
	},
	modaryImportPath + "/internal/safeerr": {
		modaryImportPath + "/action":            true,
		modaryImportPath + "/database":          true,
		modaryImportPath + "/module":            true,
		modaryImportPath + "/appcmd":            true,
		modaryImportPath + "/projecttool":       true,
		modaryImportPath + "/transport/httpapi": true,
		modaryImportPath + "/adapters/sqlite":   true,
	},
	modaryImportPath + "/internal/sqlpolicy": {
		modaryImportPath + "/adapters/sqlite": true,
	},
	modaryImportPath + "/internal/transactionoutcome": {
		modaryImportPath + "/adapters/sqlite": true,
	},
}

func TestProductionImportsFollowArchitectureLayers(t *testing.T) {
	t.Parallel()
	root := qualityRepositoryRoot(t)
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if name != root && excludedArchitectureDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		relativeDirectory, err := filepath.Rel(root, filepath.Dir(name))
		if err != nil {
			return err
		}
		importer := modaryImportPath
		if relativeDirectory != "." {
			importer += "/" + filepath.ToSlash(relativeDirectory)
		}
		if isPublicProductionPackage(importer) && packageArchitectureLayer(importer) == layerUnknown {
			t.Errorf("%s: public production package %s is missing from the explicit architecture inventory", filepath.ToSlash(name), importer)
			return nil
		}
		file, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, declaration := range file.Imports {
			imported, err := strconv.Unquote(declaration.Path.Value)
			if err != nil {
				return err
			}
			if violation := directImportViolation(importer, imported); violation != "" {
				position := fset.Position(declaration.Pos())
				t.Errorf("%s:%d: %s", filepath.ToSlash(name), position.Line, violation)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect production import directions: %v", err)
	}
}

func TestDirectImportPolicyFixturesDetectLayerAndPrivilegeViolations(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name     string
		importer string
		imported string
		rejected bool
	}{
		{
			name:     "kernel cannot import appkit",
			importer: modaryImportPath + "/module",
			imported: modaryImportPath + "/appkit",
			rejected: true,
		},
		{
			name:     "appkit cannot import transport",
			importer: modaryImportPath + "/appkit",
			imported: modaryImportPath + "/transport/httpapi",
			rejected: true,
		},
		{
			name:     "transport may import appkit",
			importer: modaryImportPath + "/transport/httpapi",
			imported: modaryImportPath + "/appkit",
		},
		{
			name:     "adapter cannot import project tooling",
			importer: modaryImportPath + "/adapters/sqlite",
			imported: modaryImportPath + "/projecttool",
			rejected: true,
		},
		{
			name:     "project tooling may inspect appkit definitions",
			importer: modaryImportPath + "/projecttool",
			imported: modaryImportPath + "/appkit",
		},
		{
			name:     "official adapter may use module assembly",
			importer: modaryImportPath + "/adapters/rbac",
			imported: modaryImportPath + "/internal/moduleassembly",
		},
		{
			name:     "other package cannot use module assembly",
			importer: modaryImportPath + "/transport/httpapi",
			imported: modaryImportPath + "/internal/moduleassembly",
			rejected: true,
		},
		{
			name:     "appcmd cannot import transport",
			importer: modaryImportPath + "/appcmd",
			imported: modaryImportPath + "/transport/httpapi",
			rejected: true,
		},
		{
			name:     "appcmd cannot import project tooling",
			importer: modaryImportPath + "/appcmd",
			imported: modaryImportPath + "/projecttool",
			rejected: true,
		},
		{
			name:     "adapter cannot import sibling adapter",
			importer: modaryImportPath + "/adapters/rbac",
			imported: modaryImportPath + "/adapters/sqlite",
			rejected: true,
		},
		{
			name:     "public package cannot import unapproved internal package",
			importer: modaryImportPath + "/appkit",
			imported: modaryImportPath + "/internal/runtimecontrol",
			rejected: true,
		},
		{
			name:     "action may import JSON Schema wrapper",
			importer: modaryImportPath + "/action",
			imported: modaryImportPath + "/internal/jsonschema",
		},
		{
			name:     "action cannot bypass JSON Schema wrapper",
			importer: modaryImportPath + "/action",
			imported: modaryImportPath + "/internal/jsonschema/engine",
			rejected: true,
		},
		{
			name:     "transport cannot bypass JSON Schema wrapper",
			importer: modaryImportPath + "/transport/httpapi",
			imported: modaryImportPath + "/internal/jsonschema/engine",
			rejected: true,
		},
		{
			name:     "unknown public importer fails closed",
			importer: modaryImportPath + "/future",
			imported: modaryImportPath + "/action",
			rejected: true,
		},
		{
			name:     "unlisted root package importer fails closed",
			importer: modaryImportPath,
			imported: modaryImportPath + "/action",
			rejected: true,
		},
		{
			name:     "unknown public target fails closed",
			importer: modaryImportPath + "/appkit",
			imported: modaryImportPath + "/future",
			rejected: true,
		},
		{
			name:     "unlisted root package target fails closed",
			importer: modaryImportPath + "/appkit",
			imported: modaryImportPath,
			rejected: true,
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			violation := directImportViolation(fixture.importer, fixture.imported)
			if fixture.rejected && violation == "" {
				t.Fatal("fixture was not rejected")
			}
			if !fixture.rejected && violation != "" {
				t.Fatalf("fixture was rejected: %s", violation)
			}
		})
	}
}

func TestPrivilegedInternalAllowlistMatchesProductionImports(t *testing.T) {
	t.Parallel()
	actual := collectPublicInternalImports(t)
	for imported, importers := range privilegedInternalImporters {
		for importer := range importers {
			if !actual[imported][importer] {
				t.Errorf("stale privileged-internal allowlist pair: %s -> %s has no production import, including build-tagged files", importer, imported)
			}
		}
	}
}

func collectPublicInternalImports(t *testing.T) map[string]map[string]bool {
	t.Helper()
	root := qualityRepositoryRoot(t)
	fset := token.NewFileSet()
	actual := make(map[string]map[string]bool)
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if name != root && excludedArchitectureDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		relativeDirectory, err := filepath.Rel(root, filepath.Dir(name))
		if err != nil {
			return err
		}
		importer := modaryImportPath
		if relativeDirectory != "." {
			importer += "/" + filepath.ToSlash(relativeDirectory)
		}
		if packageArchitectureLayer(importer) == layerUnknown {
			return nil
		}
		file, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, declaration := range file.Imports {
			imported, err := strconv.Unquote(declaration.Path.Value)
			if err != nil {
				return err
			}
			if !isInternalPackage(imported) {
				continue
			}
			if actual[imported] == nil {
				actual[imported] = make(map[string]bool)
			}
			actual[imported][importer] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("collect public production internal imports: %v", err)
	}
	return actual
}

func directImportViolation(importer, imported string) string {
	if !isModaryPackage(imported) {
		return ""
	}
	if isInternalPackage(importer) {
		return ""
	}
	source := packageArchitectureLayer(importer)
	if isPublicProductionPackage(importer) && source == layerUnknown {
		return fmt.Sprintf("public production package %s is missing from the explicit architecture inventory", importer)
	}
	target := packageArchitectureLayer(imported)
	if isPublicProductionPackage(imported) && target == layerUnknown {
		return fmt.Sprintf("public production package %s is missing from the explicit architecture inventory", imported)
	}
	if isInternalPackage(imported) {
		if privilegedInternalImporters[imported][importer] {
			return ""
		}
		return fmt.Sprintf("internal package %s is not allowlisted for public package %s", imported, importer)
	}
	if source == layerUnknown || target == layerUnknown {
		return ""
	}
	if source == layerAdapter && target == layerAdapter && importer != imported {
		return fmt.Sprintf("adapter package %s must not import sibling adapter package %s", importer, imported)
	}
	if allowedDirectLayerImports[source][target] {
		return ""
	}
	return fmt.Sprintf("%s package %s must not import %s package %s", source, importer, target, imported)
}

func packageArchitectureLayer(importPath string) architectureLayer {
	if layer, ok := publicProductionPackages[importPath]; ok {
		return layer
	}
	return layerUnknown
}

func isModaryPackage(importPath string) bool {
	return importPath == modaryImportPath || strings.HasPrefix(importPath, modaryImportPath+"/")
}

func isInternalPackage(importPath string) bool {
	return strings.Contains(importPath, "/internal/")
}

func isPublicProductionPackage(importPath string) bool {
	if !isModaryPackage(importPath) || isInternalPackage(importPath) {
		return false
	}
	for _, excluded := range []string{
		modaryImportPath + "/testdata",
		modaryImportPath + "/scripts",
	} {
		if importPath == excluded || strings.HasPrefix(importPath, excluded+"/") {
			return false
		}
	}
	return true
}

func excludedArchitectureDirectory(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "testdata"
}

func qualityRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve quality test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
}
