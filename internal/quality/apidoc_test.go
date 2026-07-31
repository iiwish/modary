package quality

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPublicDeclarationsHaveDocumentation(t *testing.T) {
	t.Parallel()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve quality test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	fset := token.NewFileSet()
	packageDocs := make(map[string]bool)
	packageFiles := make(map[string]bool)
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
		file, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		directory := filepath.Dir(name)
		packageFiles[directory] = true
		packageDocs[directory] = packageDocs[directory] || file.Doc != nil
		checkPublicDeclarations(t, fset, name, file)
		return nil
	})
	if err != nil {
		t.Fatalf("inspect public API: %v", err)
	}
	for directory := range packageFiles {
		if !packageDocs[directory] {
			t.Errorf("%s: package has no documentation comment", filepath.ToSlash(directory))
		}
	}
}

func excludedPublicAPIDirectory(root, name string) bool {
	relative, err := filepath.Rel(root, name)
	if err != nil {
		return true
	}
	for _, component := range strings.Split(filepath.ToSlash(relative), "/") {
		if component == "internal" || component == "testdata" || component == "vendor" || strings.HasPrefix(component, ".") {
			return true
		}
	}
	return false
}

func TestPublicCallableIncludesExportedMethods(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", `package fixture
type privateReceiver struct{}
type PublicReceiver struct{}
func ExportedFunction() {}
func unexportedFunction() {}
func (privateReceiver) ExportedPrivateReceiverMethod() {}
func (PublicReceiver) ExportedPublicReceiverMethod() {}
func (PublicReceiver) unexportedMethod() {}
`, 0)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	want := map[string]bool{
		"ExportedFunction":              true,
		"unexportedFunction":            false,
		"ExportedPrivateReceiverMethod": false,
		"ExportedPublicReceiverMethod":  true,
		"unexportedMethod":              false,
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if got := publicCallable(function); got != want[function.Name.Name] {
			t.Errorf("publicCallable(%s) = %t, want %t", function.Name.Name, got, want[function.Name.Name])
		}
	}
}

func checkPublicDeclarations(t *testing.T, fset *token.FileSet, name string, file *ast.File) {
	t.Helper()
	for _, declaration := range file.Decls {
		switch value := declaration.(type) {
		case *ast.FuncDecl:
			if publicCallable(value) && value.Doc == nil {
				reportMissingDoc(t, fset, name, value.Pos(), value.Name.Name)
			}
		case *ast.GenDecl:
			for _, raw := range value.Specs {
				switch spec := raw.(type) {
				case *ast.TypeSpec:
					if ast.IsExported(spec.Name.Name) && value.Doc == nil && spec.Doc == nil {
						reportMissingDoc(t, fset, name, spec.Pos(), spec.Name.Name)
					}
				case *ast.ValueSpec:
					for _, identifier := range spec.Names {
						if ast.IsExported(identifier.Name) && value.Doc == nil && spec.Doc == nil {
							reportMissingDoc(t, fset, name, identifier.Pos(), identifier.Name)
						}
					}
				}
			}
		}
	}
}

func publicCallable(function *ast.FuncDecl) bool {
	if !ast.IsExported(function.Name.Name) {
		return false
	}
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return true
	}
	return ast.IsExported(receiverTypeName(function.Recv.List[0].Type))
}

func receiverTypeName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return receiverTypeName(value.X)
	case *ast.IndexExpr:
		return receiverTypeName(value.X)
	case *ast.IndexListExpr:
		return receiverTypeName(value.X)
	default:
		return ""
	}
}

func reportMissingDoc(t *testing.T, fset *token.FileSet, name string, position token.Pos, symbol string) {
	t.Helper()
	location := fset.Position(position)
	t.Errorf("%s:%d: exported declaration %s has no documentation comment", filepath.ToSlash(name), location.Line, symbol)
}
