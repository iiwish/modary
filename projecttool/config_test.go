package projecttool

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseManifestAcceptsOnlyStrictOutputOnlyDocument(t *testing.T) {
	manifest, err := ParseManifest([]byte(validProjectManifest))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if manifest.Application != fixtureMetadata() {
		t.Fatalf("application = %#v", manifest.Application)
	}
	if manifest.Outputs.Graph != "internal/generated/module_graph.json" ||
		manifest.Outputs.Actions != "internal/generated/action_catalog.json" ||
		manifest.Outputs.TypeScript != "web/src/generated/actionContracts.ts" {
		t.Fatalf("outputs = %#v", manifest.Outputs)
	}
	if manifest.Build.Package != "./cmd/example-app" || manifest.Build.Output != "dist/example-app" {
		t.Fatalf("build = %#v", manifest.Build)
	}
}

func TestParseManifestRejectsMalformedAndExtendedYAML(t *testing.T) {
	var deeplyNested strings.Builder
	for depth := 0; depth < maximumYAMLDepth+2; depth++ {
		deeplyNested.WriteString(strings.Repeat("  ", depth))
		deeplyNested.WriteString("value:\n")
	}
	deeplyNested.WriteString(strings.Repeat("  ", maximumYAMLDepth+2))
	deeplyNested.WriteString("terminal\n")
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "invalid UTF-8", data: []byte{0xff}},
		{name: "multiple documents", data: []byte(validProjectManifest + "---\napplication: {}\n")},
		{name: "empty second document", data: []byte(validProjectManifest + "---\n")},
		{name: "top-level modules", data: []byte(validProjectManifest + "modules: []\n")},
		{name: "unknown nested field", data: []byte(strings.Replace(validProjectManifest, "  name: Example App", "  name: Example App\n  owner: example", 1))},
		{name: "duplicate field", data: []byte(strings.Replace(validProjectManifest, "  id: example-app", "  id: example-app\n  id: other-app", 1))},
		{name: "anchor", data: []byte(strings.Replace(validProjectManifest, "  id: example-app", "  id: &application-id example-app", 1))},
		{name: "alias", data: []byte(strings.Replace(validProjectManifest, "  name: Example App", "  name: &name Example App\n  version: *name", 1))},
		{name: "oversized", data: append([]byte(validProjectManifest), make([]byte, MaximumManifestBytes)...)},
		{name: "excessive YAML depth", data: []byte(deeplyNested.String())},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseManifest(test.data); err == nil {
				t.Fatal("ParseManifest succeeded")
			}
		})
	}
}

func TestParseManifestRejectsUnsafeAndAliasedPaths(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
	}{
		{name: "absolute", old: "internal/generated/module_graph.json", new: "/tmp/graph.json"},
		{name: "traversal", old: "internal/generated/module_graph.json", new: "../graph.json"},
		{name: "embedded traversal", old: "internal/generated/module_graph.json", new: "internal/../graph.json"},
		{name: "backslash", old: "internal/generated/module_graph.json", new: `internal\graph.json`},
		{name: "Windows volume", old: "internal/generated/module_graph.json", new: "C:/graph.json"},
		{name: "Windows alternate stream", old: "internal/generated/module_graph.json", new: "internal/graph.json:stream"},
		{name: "Windows less than", old: "internal/generated/module_graph.json", new: "internal/graph<.json"},
		{name: "Windows greater than", old: "internal/generated/module_graph.json", new: "internal/graph>.json"},
		{name: "Windows quote", old: "internal/generated/module_graph.json", new: `internal/graph".json`},
		{name: "Windows pipe", old: "internal/generated/module_graph.json", new: "internal/graph|.json"},
		{name: "Windows question", old: "internal/generated/module_graph.json", new: "internal/graph?.json"},
		{name: "Windows wildcard", old: "internal/generated/module_graph.json", new: "internal/graph*.json"},
		{name: "Windows device", old: "internal/generated/module_graph.json", new: "internal/CON.json"},
		{name: "Unicode NFC", old: "internal/generated/module_graph.json", new: "internal/caf\u00e9.json"},
		{name: "Unicode NFD", old: "internal/generated/module_graph.json", new: "internal/cafe\u0301.json"},
		{name: "Unicode fullwidth alias", old: "internal/generated/module_graph.json", new: "internal/\uff47raph.json"},
		{name: "space", old: "internal/generated/module_graph.json", new: "internal/generated graph.json"},
		{name: "trailing dot", old: "internal/generated/module_graph.json", new: "internal/generated./graph.json"},
		{name: "oversized component", old: "internal/generated/module_graph.json", new: "internal/" + strings.Repeat("x", maximumPathComponentLen+1)},
		{name: "noncanonical duplicate slash", old: "internal/generated/module_graph.json", new: "internal//graph.json"},
		{name: "surrounding whitespace", old: "internal/generated/module_graph.json", new: `" internal/graph.json"`},
		{name: "manifest alias", old: "internal/generated/module_graph.json", new: "MODARY.YAML"},
		{name: "exact output alias", old: "internal/generated/action_catalog.json", new: "internal/generated/module_graph.json"},
		{name: "case-folded output alias", old: "internal/generated/action_catalog.json", new: "INTERNAL/GENERATED/MODULE_GRAPH.JSON"},
		{name: "hierarchical output alias", old: "internal/generated/action_catalog.json", new: "internal/generated/module_graph.json/child"},
		{name: "build output alias", old: "dist/example-app", new: "web/src/generated"},
		{name: "absolute build package", old: "./cmd/example-app", new: "/cmd/example-app"},
		{name: "root build package", old: "./cmd/example-app", new: "./"},
		{name: "recursive build package", old: "./cmd/example-app", new: "./cmd/..."},
		{name: "output equals build package", old: "internal/generated/module_graph.json", new: "cmd/example-app"},
		{name: "output contains build package", old: "internal/generated/module_graph.json", new: "cmd"},
		{name: "build package contains output", old: "./cmd/example-app", new: "./internal"},
		{name: "build output contains package", old: "dist/example-app", new: "cmd"},
		{name: "build output inside package", old: "dist/example-app", new: "cmd/example-app/binary"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := strings.Replace(validProjectManifest, test.old, test.new, 1)
			if _, err := ParseManifest([]byte(data)); err == nil {
				t.Fatalf("ParseManifest accepted %q", test.new)
			}
		})
	}
}

func TestLoadRejectsSymlinkNonRegularAndHardlinkAliases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink and hardlink semantics differ on Windows")
	}
	t.Run("manifest symlink", func(t *testing.T) {
		parent := t.TempDir()
		root := filepath.Join(parent, "project")
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
		realManifest := filepath.Join(parent, "real.yaml")
		if err := os.WriteFile(realManifest, []byte(validProjectManifest), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(realManifest, filepath.Join(root, ProjectManifestName)); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(root); err == nil {
			t.Fatal("Load accepted a manifest symlink")
		}
	})

	t.Run("output parent symlink", func(t *testing.T) {
		root := writeFixtureProject(t, validProjectManifest)
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, "internal")); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(root); err == nil {
			t.Fatal("Load accepted a symlinked output parent")
		}
	})

	t.Run("build package symlink", func(t *testing.T) {
		root := writeFixtureProject(t, validProjectManifest)
		if err := os.MkdirAll(filepath.Join(root, "cmd"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(t.TempDir(), filepath.Join(root, "cmd", "example-app")); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(root); err == nil {
			t.Fatal("Load accepted a symlinked build package")
		}
	})

	t.Run("output is directory", func(t *testing.T) {
		root := writeFixtureProject(t, validProjectManifest)
		if err := os.MkdirAll(filepath.Join(root, "dist", "example-app"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(root); err == nil {
			t.Fatal("Load accepted a directory output")
		}
	})

	t.Run("outputs hardlinked", func(t *testing.T) {
		manifest := strings.Replace(validProjectManifest, "internal/generated/module_graph.json", "generated/a.json", 1)
		manifest = strings.Replace(manifest, "internal/generated/action_catalog.json", "generated/b.json", 1)
		root := writeFixtureProject(t, manifest)
		if err := os.MkdirAll(filepath.Join(root, "generated"), 0o755); err != nil {
			t.Fatal(err)
		}
		first := filepath.Join(root, "generated", "a.json")
		if err := os.WriteFile(first, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(first, filepath.Join(root, "generated", "b.json")); err != nil {
			t.Skipf("hardlinks unavailable: %v", err)
		}
		if _, err := Load(root); err == nil {
			t.Fatal("Load accepted hardlinked outputs")
		}
	})

	t.Run("manifest hardlinked as output", func(t *testing.T) {
		manifest := strings.Replace(validProjectManifest, "internal/generated/module_graph.json", "generated/graph.json", 1)
		root := writeFixtureProject(t, manifest)
		if err := os.MkdirAll(filepath.Join(root, "generated"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(filepath.Join(root, ProjectManifestName), filepath.Join(root, "generated", "graph.json")); err != nil {
			t.Skipf("hardlinks unavailable: %v", err)
		}
		if _, err := Load(root); err == nil {
			t.Fatal("Load accepted a manifest hardlink output")
		}
	})
}

func TestProjectRejectsRootManifestAndOutputChangesAfterLoad(t *testing.T) {
	t.Run("manifest changed", func(t *testing.T) {
		root := writeFixtureProject(t, validProjectManifest)
		project := loadFixtureProject(t, root)
		if err := os.WriteFile(filepath.Join(root, ProjectManifestName), []byte(validProjectManifest+"# changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		counters := &inspectionCounters{}
		if _, err := project.Verify(fixtureDefinition(counters, false)); err == nil {
			t.Fatal("Verify accepted a changed manifest")
		}
		assertNoInspectionSideEffects(t, counters)
	})

	t.Run("root replaced", func(t *testing.T) {
		parent := t.TempDir()
		root := filepath.Join(parent, "project")
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ProjectManifestName), []byte(validProjectManifest), 0o644); err != nil {
			t.Fatal(err)
		}
		project := loadFixtureProject(t, root)
		moved := filepath.Join(parent, "moved")
		if err := os.Rename(root, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ProjectManifestName), []byte(validProjectManifest), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := project.Verify(fixtureDefinition(&inspectionCounters{}, false)); err == nil {
			t.Fatal("Verify accepted a replaced root")
		}
	})

	t.Run("output becomes symlink", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlink semantics differ on Windows")
		}
		root := writeFixtureProject(t, validProjectManifest)
		project := loadFixtureProject(t, root)
		outside := filepath.Join(t.TempDir(), "outside")
		if err := os.WriteFile(outside, []byte("untouched"), 0o644); err != nil {
			t.Fatal(err)
		}
		graph := project.Manifest().Outputs.Graph
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, graph)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, graph)); err != nil {
			t.Fatal(err)
		}
		_, err := project.Generate(fixtureDefinition(&inspectionCounters{}, false))
		if err == nil {
			t.Fatal("Generate accepted a symlink output")
		}
		data, readErr := os.ReadFile(outside)
		if readErr != nil || string(data) != "untouched" {
			t.Fatalf("outside file changed: %q, %v", data, readErr)
		}
		assertNoTemporaryArtifacts(t, root)
	})
}

func TestLoadCanonicalizesRootSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	root := writeFixtureProject(t, validProjectManifest)
	link := filepath.Join(t.TempDir(), "project-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	project, err := Load(link)
	if err != nil {
		t.Fatalf("Load through root symlink: %v", err)
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if project.Root() != canonical {
		t.Fatalf("Root = %q, want %q", project.Root(), canonical)
	}
}

func TestUsageErrorsRemainInspectable(t *testing.T) {
	err := newUsageError("bad input")
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("errors.Is(ErrUsage) = false: %v", err)
	}
	if err.Error() != "bad input" {
		t.Fatalf("usage error = %q", err.Error())
	}
}

func TestNilProjectReceiversFailClosed(t *testing.T) {
	var project *Project
	if project.Root() != "" || project.Manifest() != (Manifest{}) {
		t.Fatal("nil Project accessors returned non-zero values")
	}
	definition := fixtureDefinition(&inspectionCounters{}, false)
	if _, err := project.Verify(definition); err == nil {
		t.Fatal("nil Project Verify succeeded")
	}
	if _, err := project.Generate(definition); err == nil {
		t.Fatal("nil Project Generate succeeded")
	}
	if _, err := project.Check(definition); err == nil {
		t.Fatal("nil Project Check succeeded")
	}
}

func FuzzParseManifestFailsClosed(f *testing.F) {
	f.Add([]byte(validProjectManifest))
	f.Add([]byte("modules: []\n"))
	f.Add([]byte("application: &app {id: example-app}\ncopy: *app\n"))
	f.Add([]byte{0xff, 0x00, 0x01})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseManifest(data)
	})
}
