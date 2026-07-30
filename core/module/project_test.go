package module

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedRegistryTracksModuleCompositionDeterministically(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "modules/database/module.yaml", moduleYAML("database", "adapter", nil, []string{"database"}, nil))
	writeProjectFile(t, root, "modules/api/module.yaml", moduleYAML("api", "feature", []string{"database"}, []string{"api"}, []string{"demo.list"}))
	writeProjectFile(t, root, "modules/console/module.yaml", `schemaVersion: modary.module/v1alpha1
id: console
version: 0.1.0
type: feature
requires: [api]
provides: [console]
ui:
  routes:
    - id: console.home
      path: /
      entry: ./ui/routes.tsx
`)
	writeProjectFile(t, root, "modules/agent/module.yaml", moduleYAML("agent", "adapter", []string{"api"}, []string{"agent"}, nil))
	writeProjectFile(t, root, "modary.yaml", "app:\n  id: fixture\nmodules: [database, api, console, agent]\n")

	project, err := LoadProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := project.Generate(); err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(root, "internal", "generated", "modules_gen.go")
	withAgent, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(withAgent, []byte(`"modary/modules/agent"`)) {
		t.Fatalf("generated registry does not include selected agent module:\n%s", withAgent)
	}
	routes, err := os.ReadFile(filepath.Join(root, "web", "src", "generated", "routes.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(routes, []byte(`../../../modules/console/ui/routes`)) || !bytes.Contains(routes, []byte(`consoleRoutes["console.home"]`)) {
		t.Fatalf("generated UI registry does not load the declared entry:\n%s", routes)
	}
	if err := project.Generate(); err != nil {
		t.Fatal(err)
	}
	repeated, _ := os.ReadFile(registryPath)
	if !bytes.Equal(withAgent, repeated) {
		t.Fatal("repeated generation produced a semantic diff")
	}

	writeProjectFile(t, root, "modary.yaml", "app:\n  id: fixture\nmodules: [database, api, console]\n")
	withoutAgentProject, err := LoadProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := withoutAgentProject.Generate(); err != nil {
		t.Fatal(err)
	}
	withoutAgent, _ := os.ReadFile(registryPath)
	if bytes.Contains(withoutAgent, []byte(`"modary/modules/agent"`)) {
		t.Fatalf("removed module remains in generated registry:\n%s", withoutAgent)
	}
	if !bytes.Contains(withoutAgent, []byte(`"modary/modules/console"`)) || !bytes.Contains(withoutAgent, []byte(`"modary/modules/api"`)) {
		t.Fatalf("UI/API modules were lost with agent removal:\n%s", withoutAgent)
	}
}

func TestLoadProjectReportsMissingSelectedProvider(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "modules/api/module.yaml", moduleYAML("api", "feature", []string{"database"}, []string{"api"}, nil))
	writeProjectFile(t, root, "modary.yaml", "app:\n  id: fixture\nmodules: [api]\n")
	_, err := LoadProject(root)
	if err == nil || !strings.Contains(err.Error(), `module api requires missing capability "database"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestSourceBoundaryRejectsCoreImportingFeatureModule(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "core/leak.go", "package core\nimport _ \"modary/modules/rulary-core\"\n")
	err := VerifySourceBoundaries(root)
	if err == nil || !strings.Contains(err.Error(), "module boundary violation") {
		t.Fatalf("error = %v", err)
	}
}

func TestCoreContainsNoF0DomainTerms(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	terms := [][]byte{[]byte("RuleSet"), []byte("RuleSpec"), []byte("Fluxale Spec"), []byte("rulary")}
	err := filepath.WalkDir(filepath.Join(root, "core"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, term := range terms {
			if bytes.Contains(data, term) {
				t.Errorf("core domain leak: %s contains %q", path, term)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func moduleYAML(id, kind string, requires, provides, actions []string) string {
	value := "schemaVersion: modary.module/v1alpha1\nid: " + id + "\nversion: 0.1.0\ntype: " + kind + "\n"
	if len(requires) > 0 {
		value += "requires: [" + strings.Join(requires, ", ") + "]\n"
	}
	if len(provides) > 0 {
		value += "provides: [" + strings.Join(provides, ", ") + "]\n"
	}
	if len(actions) > 0 {
		value += "actions: [" + strings.Join(actions, ", ") + "]\n"
	}
	return value
}

func writeProjectFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
