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

func TestPublicPackagesDoNotReintroducePrivilegedAssemblySymbols(t *testing.T) {
	t.Parallel()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve quality test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	forbidden := map[string]map[string]struct{}{
		"action": {
			"Registry": {}, "NewRegistry": {},
			"ErrPlanNotFound": {}, "PlanStore": {},
			"IdempotencyStatus": {}, "IdempotencyRunning": {}, "IdempotencyCompleted": {},
			"IdempotencyRecord": {}, "IdempotencyStore": {},
			"ValidatePlanRecord": {}, "ValidateIdempotencyLookupRecord": {},
			"ValidateIdempotencyReservationRecord": {}, "ValidateIdempotencyCompletionRecord": {},
			"ValidateStoredIdempotencyRecord": {}, "ErrTransactionManagerContract": {},
		},
		"database": {
			"Backend": {}, "Control": {}, "NewControl": {}, "ApplyMigrations": {},
		},
		"module": {
			"ProvideDatabase": {}, "ResolveDatabaseControl": {}, "ResolveHostDatabaseControl": {},
			"ResolveHostService": {}, "Host.NewRuntime": {},
		},
	}
	for packageName, symbols := range forbidden {
		packageName, symbols := packageName, symbols
		t.Run(packageName, func(t *testing.T) {
			t.Parallel()
			packages, err := parser.ParseDir(token.NewFileSet(), filepath.Join(root, packageName), func(info os.FileInfo) bool {
				return filepath.Ext(info.Name()) == ".go" && !strings.HasSuffix(info.Name(), "_test.go")
			}, 0)
			if err != nil {
				t.Fatal(err)
			}
			parsed, ok := packages[packageName]
			if !ok {
				t.Fatalf("package %s was not parsed", packageName)
			}
			for _, file := range parsed.Files {
				for _, declaration := range file.Decls {
					for _, symbol := range exportedDeclarationNames(declaration) {
						if _, rejected := symbols[symbol]; rejected {
							t.Errorf("public package %s exposes privileged symbol %s", packageName, symbol)
						}
					}
				}
			}
		})
	}
}

func exportedDeclarationNames(declaration ast.Decl) []string {
	switch value := declaration.(type) {
	case *ast.FuncDecl:
		if !publicCallable(value) {
			return nil
		}
		if value.Recv == nil || len(value.Recv.List) == 0 {
			return []string{value.Name.Name}
		}
		return []string{receiverTypeName(value.Recv.List[0].Type) + "." + value.Name.Name}
	case *ast.GenDecl:
		var names []string
		for _, raw := range value.Specs {
			switch spec := raw.(type) {
			case *ast.TypeSpec:
				if ast.IsExported(spec.Name.Name) {
					names = append(names, spec.Name.Name)
				}
			case *ast.ValueSpec:
				for _, name := range spec.Names {
					if ast.IsExported(name.Name) {
						names = append(names, name.Name)
					}
				}
			}
		}
		return names
	}
	return nil
}
