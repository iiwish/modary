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

type recoverRetentionSpec struct {
	name     string
	operator token.Token
}

type retainedRecover struct {
	identifier *ast.Ident
	assignment *ast.AssignStmt
	call       *ast.CallExpr
}

func TestProductionPanicDetectionDoesNotDependOnRecoveredValue(t *testing.T) {
	t.Parallel()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve quality test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	allowed := map[string]recoverRetentionSpec{
		"components/governedpostgres/executor.go": {name: "recovered", operator: token.DEFINE},
		"internal/databasecontrol/transaction.go": {name: "panicValue", operator: token.ASSIGN},
	}
	seen := make(map[string]int, len(allowed))
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if name != root && (entry.Name() == ".git" || entry.Name() == "vendor") {
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
		relative, relativeErr := filepath.Rel(root, name)
		if relativeErr != nil {
			return relativeErr
		}
		relative = filepath.ToSlash(relative)
		inspectRecoverUsage(t, fset, relative, file, allowed, seen)
		return nil
	})
	if err != nil {
		t.Fatalf("inspect production panic boundaries: %v", err)
	}
	for name := range allowed {
		if seen[name] != 1 {
			t.Errorf("%s: retained recover count = %d, want exactly 1", name, seen[name])
		}
	}
}

func inspectRecoverUsage(
	t *testing.T,
	fset *token.FileSet,
	name string,
	file *ast.File,
	allowed map[string]recoverRetentionSpec,
	seen map[string]int,
) {
	t.Helper()
	parents := panicBoundaryParents(file)
	var retained []retainedRecover
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !isBuiltinCall(call, "recover") {
			return true
		}
		assignment, ok := parents[call].(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || assignment.Rhs[0] != call {
			reportPanicBoundary(t, fset, name, call.Pos(), "recover result must be explicitly discarded")
			return true
		}
		identifier, ok := assignment.Lhs[0].(*ast.Ident)
		if !ok {
			reportPanicBoundary(t, fset, name, call.Pos(), "recover result must not escape through a compound assignment")
			return true
		}
		if identifier.Name == "_" {
			if assignment.Tok != token.ASSIGN {
				reportPanicBoundary(t, fset, name, call.Pos(), "discarded recover must use _ = recover()")
			}
			return true
		}
		spec, permitted := allowed[name]
		if !permitted || identifier.Name != spec.name || assignment.Tok != spec.operator {
			reportPanicBoundary(t, fset, name, call.Pos(), "recover result may only be retained by an approved exact rethrow boundary")
			return true
		}
		seen[name]++
		retained = append(retained, retainedRecover{identifier: identifier, assignment: assignment, call: call})
		return true
	})

	for _, recovery := range retained {
		checkExactRecoverRethrow(t, fset, name, file, parents, recovery)
	}
}

func checkExactRecoverRethrow(
	t *testing.T,
	fset *token.FileSet,
	name string,
	file *ast.File,
	parents map[ast.Node]ast.Node,
	recovery retainedRecover,
) {
	t.Helper()
	var rethrows int
	ast.Inspect(file, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || !sameRecoverIdentifier(identifier, recovery.identifier) {
			return true
		}
		if identifier == recovery.identifier || isRecoverDeclarationIdentifier(identifier, parents[identifier]) {
			return true
		}
		parent := parents[identifier]
		if parent == recovery.assignment {
			return true
		}
		call, ok := parent.(*ast.CallExpr)
		if ok && isBuiltinCall(call, "panic") && len(call.Args) == 1 && call.Args[0] == identifier {
			rethrows++
			return true
		}
		if binary, ok := parent.(*ast.BinaryExpr); ok &&
			(binary.Op == token.EQL || binary.Op == token.NEQ) &&
			(isNilIdentifier(binary.X) || isNilIdentifier(binary.Y)) {
			reportPanicBoundary(t, fset, name, identifier.Pos(), "panic detection must not compare a recovered value with nil")
			return true
		}
		reportPanicBoundary(t, fset, name, identifier.Pos(), "retained recover result may only be passed directly to panic")
		return true
	})
	if rethrows != 1 {
		reportPanicBoundary(t, fset, name, recovery.call.Pos(), "retained recover result must have exactly one direct panic rethrow")
	}
}

func panicBoundaryParents(root ast.Node) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		return true
	})
	return parents
}

func isBuiltinCall(call *ast.CallExpr, name string) bool {
	identifier, ok := call.Fun.(*ast.Ident)
	if !ok || identifier.Name != name || identifier.Obj != nil {
		return false
	}
	switch name {
	case "recover":
		return len(call.Args) == 0
	case "panic":
		return len(call.Args) == 1
	default:
		return true
	}
}

func isRecoverDeclarationIdentifier(identifier *ast.Ident, parent ast.Node) bool {
	spec, ok := parent.(*ast.ValueSpec)
	if !ok {
		return false
	}
	for _, declared := range spec.Names {
		if declared == identifier {
			return true
		}
	}
	return false
}

func sameRecoverIdentifier(candidate, retained *ast.Ident) bool {
	if candidate == nil || retained == nil || candidate.Name != retained.Name {
		return false
	}
	if retained.Obj != nil || candidate.Obj != nil {
		return retained.Obj != nil && candidate.Obj == retained.Obj
	}
	return true
}

func isNilIdentifier(expression ast.Expr) bool {
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.Name == "nil"
}

func reportPanicBoundary(t *testing.T, fset *token.FileSet, name string, position token.Pos, message string) {
	t.Helper()
	location := fset.Position(position)
	t.Errorf("%s:%d: %s", name, location.Line, message)
}
