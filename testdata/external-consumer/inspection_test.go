package consumer_test

import (
	"context"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"

	"example.com/modary-counter-consumer/internal/project"
	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/projecttool"
)

type forbiddenFS struct{ opens *atomic.Int64 }

func (filesystem forbiddenFS) Open(string) (fs.File, error) {
	filesystem.opens.Add(1)
	panic("project inspection opened a migration filesystem")
}

func TestProjectToolingIsPureDeterministicAndDetectsDrift(t *testing.T) {
	root := t.TempDir()
	manifest, err := os.ReadFile(filepath.Join(consumerRoot(t), projecttool.ProjectManifestName))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, projecttool.ProjectManifestName), manifest, 0o644); err != nil {
		t.Fatal(err)
	}

	var starts atomic.Int64
	var handlers atomic.Int64
	var opens atomic.Int64
	definition := failOnRuntimeUse(mustDefinition(t, project.Config{
		DatabasePath: filepath.Join(root, "must-not-exist.db"),
	}), &starts, &handlers, &opens)

	loaded, err := projecttool.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	first, err := loaded.Verify(definition)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if len(first.Modules) != 6 || len(first.Actions) != 1 {
		t.Fatalf("Verify() snapshot = %d modules, %d Actions", len(first.Modules), len(first.Actions))
	}
	generated, err := loaded.Generate(definition)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(generated.Written) != 3 {
		t.Fatalf("Generate() written = %#v", generated.Written)
	}
	firstBytes := readGeneratedArtifacts(t, root)

	second, err := loaded.Generate(definition)
	if err != nil {
		t.Fatalf("second Generate() error = %v", err)
	}
	if len(second.Written) != 0 || len(second.Unchanged) != 3 {
		t.Fatalf("second Generate() = %#v", second)
	}
	if got := readGeneratedArtifacts(t, root); !reflect.DeepEqual(got, firstBytes) {
		t.Fatal("repeated generation was not byte-identical")
	}
	if drift, err := loaded.Check(definition); err != nil || len(drift) != 0 {
		t.Fatalf("Check(current) = %#v, %v", drift, err)
	}

	actionPath := filepath.Join(root, "internal", "generated", "action_catalog.json")
	const corrupted = "consumer-owned drift\n"
	if err := os.WriteFile(actionPath, []byte(corrupted), 0o644); err != nil {
		t.Fatal(err)
	}
	drift, err := loaded.Check(definition)
	if err != nil {
		t.Fatalf("Check(drift) error = %v", err)
	}
	if len(drift) != 1 || drift[0].Path != "internal/generated/action_catalog.json" ||
		drift[0].Status != projecttool.DriftDifferent {
		t.Fatalf("Check(drift) = %#v", drift)
	}
	after, err := os.ReadFile(actionPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != corrupted {
		t.Fatal("check mode modified the drifted artifact")
	}
	if starts.Load() != 0 || handlers.Load() != 0 || opens.Load() != 0 {
		t.Fatalf("inspection side effects: starts=%d handlers=%d opens=%d",
			starts.Load(), handlers.Load(), opens.Load())
	}
	if _, err := os.Stat(filepath.Join(root, "must-not-exist.db")); !os.IsNotExist(err) {
		t.Fatalf("inspection created database path: %v", err)
	}
}

func TestConsumerImportsOnlyPublicCanonicalModaryPackages(t *testing.T) {
	err := filepath.WalkDir(consumerRoot(t), func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(name) != ".go" {
			return nil
		}
		source, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range source.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			const framework = "github.com/iiwish/modary/"
			for _, segment := range []string{"internal/", "core/", "modules/", "web/", "tests/", "testdata/"} {
				forbidden := framework + segment
				if len(path) >= len(forbidden) && path[:len(forbidden)] == forbidden {
					t.Errorf("%s imports forbidden framework path %q", name, path)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func failOnRuntimeUse(
	definition appkit.Definition,
	starts, handlers, opens *atomic.Int64,
) appkit.Definition {
	cloned := definition
	cloned.Modules = append([]module.Registration(nil), definition.Modules...)
	for moduleIndex := range cloned.Modules {
		registration := cloned.Modules[moduleIndex]
		registration.Start = func(context.Context, module.Scope) error {
			starts.Add(1)
			panic("project inspection started a Module")
		}
		registration.Definition.Migrations = append(
			[]module.MigrationSource(nil),
			registration.Definition.Migrations...,
		)
		for migrationIndex := range registration.Definition.Migrations {
			registration.Definition.Migrations[migrationIndex].Files = forbiddenFS{opens: opens}
		}
		registration.Definition.Actions = append(
			[]module.ActionBinding(nil),
			registration.Definition.Actions...,
		)
		for actionIndex := range registration.Definition.Actions {
			registration.Definition.Actions[actionIndex].NewHandler = func(
				context.Context,
				module.Resolver,
			) (action.Handler, error) {
				handlers.Add(1)
				panic("project inspection constructed an Action handler")
			}
		}
		cloned.Modules[moduleIndex] = registration
	}
	return cloned
}

func readGeneratedArtifacts(t *testing.T, root string) map[string]string {
	t.Helper()
	result := make(map[string]string)
	for _, name := range []string{
		"internal/generated/module_graph.json",
		"internal/generated/action_catalog.json",
		"internal/generated/action_contracts.ts",
	} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		result[name] = string(data)
	}
	return result
}

func consumerRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve external consumer source path")
	}
	return filepath.Dir(filename)
}
