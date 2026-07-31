package quality

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestPublicAPIDoesNotExposeInternalPackages(t *testing.T) {
	t.Parallel()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve quality test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if name != root && excludedPublicAPIDirectory(root, name) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, name, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		aliases := internalImportAliases(t, fset, name, file)
		checkInternalAPIReferences(t, fset, name, file, aliases)
		return nil
	})
	if err != nil {
		t.Fatalf("inspect public API boundaries: %v", err)
	}
}

func TestInternalAPIReferenceDetectionCoversPublicReachability(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", `package fixture
import private "example.com/framework/internal/private"
type privateReceiver struct{}
type PublicReceiver struct{}
func ExportedFunction(private.Value) {}
func (privateReceiver) ExportedPrivateReceiverMethod(private.Value) {}
func (PublicReceiver) ExportedPublicReceiverMethod(private.Value) {}
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	aliases := map[string]string{"private": "example.com/framework/internal/private"}
	var exposed []string
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || !publicCallable(function) || !containsInternalReference(function.Type, aliases) {
			continue
		}
		exposed = append(exposed, function.Name.Name)
	}
	want := []string{"ExportedFunction", "ExportedPublicReceiverMethod"}
	if strings.Join(exposed, ",") != strings.Join(want, ",") {
		t.Fatalf("detected public internal references = %v, want %v", exposed, want)
	}
}

func internalImportAliases(t *testing.T, fset *token.FileSet, name string, file *ast.File) map[string]string {
	t.Helper()
	aliases := make(map[string]string)
	for _, imported := range file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Errorf("%s:%d: invalid import path: %v", filepath.ToSlash(name), fset.Position(imported.Pos()).Line, err)
			continue
		}
		if !strings.Contains(path, "/internal/") {
			continue
		}
		alias := pathpkg.Base(path)
		if imported.Name != nil {
			alias = imported.Name.Name
		}
		if alias == "." {
			t.Errorf("%s:%d: public package dot-imports internal package %s", filepath.ToSlash(name), fset.Position(imported.Pos()).Line, path)
			continue
		}
		if alias != "_" {
			aliases[alias] = path
		}
	}
	return aliases
}

func checkInternalAPIReferences(t *testing.T, fset *token.FileSet, name string, file *ast.File, aliases map[string]string) {
	t.Helper()
	for _, declaration := range file.Decls {
		switch value := declaration.(type) {
		case *ast.FuncDecl:
			if publicCallable(value) && containsInternalReference(value.Type, aliases) {
				reportInternalAPIReference(t, fset, name, value.Pos(), value.Name.Name)
			}
		case *ast.GenDecl:
			for _, raw := range value.Specs {
				switch spec := raw.(type) {
				case *ast.TypeSpec:
					if ast.IsExported(spec.Name.Name) && exportedTypeContainsInternalReference(spec, aliases) {
						reportInternalAPIReference(t, fset, name, spec.Pos(), spec.Name.Name)
					}
				case *ast.ValueSpec:
					if exportedValueContainsInternalReference(spec, aliases) {
						reportInternalAPIReference(t, fset, name, spec.Pos(), firstExportedName(spec.Names))
					}
				}
			}
		}
	}
}

func exportedTypeContainsInternalReference(spec *ast.TypeSpec, aliases map[string]string) bool {
	if spec.TypeParams != nil && containsInternalReference(spec.TypeParams, aliases) {
		return true
	}
	switch value := spec.Type.(type) {
	case *ast.StructType:
		for _, field := range value.Fields.List {
			if (len(field.Names) == 0 || anyExported(field.Names)) && containsInternalReference(field.Type, aliases) {
				return true
			}
		}
		return false
	case *ast.InterfaceType:
		for _, field := range value.Methods.List {
			if (len(field.Names) == 0 || anyExported(field.Names)) && containsInternalReference(field.Type, aliases) {
				return true
			}
		}
		return false
	default:
		return containsInternalReference(spec.Type, aliases)
	}
}

func exportedValueContainsInternalReference(spec *ast.ValueSpec, aliases map[string]string) bool {
	if !anyExported(spec.Names) {
		return false
	}
	if spec.Type != nil && containsInternalReference(spec.Type, aliases) {
		return true
	}
	for _, value := range spec.Values {
		if containsInternalReference(value, aliases) {
			return true
		}
	}
	return false
}

func anyExported(names []*ast.Ident) bool {
	for _, name := range names {
		if ast.IsExported(name.Name) {
			return true
		}
	}
	return false
}

func firstExportedName(names []*ast.Ident) string {
	for _, name := range names {
		if ast.IsExported(name.Name) {
			return name.Name
		}
	}
	return "<exported value>"
}

func containsInternalReference(node ast.Node, aliases map[string]string) bool {
	if node == nil || len(aliases) == 0 {
		return false
	}
	found := false
	ast.Inspect(node, func(current ast.Node) bool {
		selector, ok := current.(*ast.SelectorExpr)
		if !ok {
			return !found
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return !found
		}
		_, found = aliases[identifier.Name]
		return !found
	})
	return found
}

func reportInternalAPIReference(t *testing.T, fset *token.FileSet, name string, position token.Pos, symbol string) {
	t.Helper()
	location := fset.Position(position)
	t.Errorf("%s:%d: exported declaration %s exposes a type from an internal package", filepath.ToSlash(name), location.Line, symbol)
}
