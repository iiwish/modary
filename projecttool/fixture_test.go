package projecttool

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/module"
)

func requireSecureBuildPlatformForTest(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("secure Build runtime is supported only on Linux and Darwin")
	}
}

const validProjectManifest = `application:
  id: example-app
  name: Example App
  version: 1.2.3
outputs:
  graph: internal/generated/module_graph.json
  actions: internal/generated/action_catalog.json
  typescript: web/src/generated/actionContracts.ts
build:
  package: ./cmd/example-app
  output: dist/example-app
`

type inspectionCounters struct {
	starts   atomic.Int64
	handlers atomic.Int64
	opens    atomic.Int64
}

type failOnOpenFS struct{ calls *atomic.Int64 }

func (filesystem failOnOpenFS) Open(string) (fs.File, error) {
	filesystem.calls.Add(1)
	panic("migration files must not be opened during inspection")
}

func fixtureDefinition(counters *inspectionCounters, reverse bool) appkit.Definition {
	storage := module.Registration{
		Definition: module.Definition{
			Manifest: module.Manifest{
				SchemaVersion: module.SchemaVersion,
				ID:            "storage",
				Version:       "1.4.0",
				Type:          module.ModuleTypeAdapter,
				Provides: []module.Capability{
					module.CapabilityDatabase,
					"records.store",
					"records.clock",
				},
			},
			Migrations: []module.MigrationSource{
				{Driver: "sqlite", Files: failOnOpenFS{calls: &counters.opens}},
				{Driver: "archive", Files: failOnOpenFS{calls: &counters.opens}},
			},
		},
		Start: func(context.Context, module.Scope) error {
			counters.starts.Add(1)
			panic("Module Start must not run during inspection")
		},
	}
	feature := module.Registration{
		Definition: module.Definition{
			Manifest: module.Manifest{
				SchemaVersion: module.SchemaVersion,
				ID:            "records",
				Version:       "2.1.0",
				Type:          module.ModuleTypeFeature,
				Requires:      []module.Capability{"records.store", "records.clock"},
				Provides:      []module.Capability{"records.commands"},
			},
			Actions: []module.ActionBinding{
				fixtureAction("records.create", true, counters),
				fixtureAction("records.archive", false, counters),
			},
		},
		Start: func(context.Context, module.Scope) error {
			counters.starts.Add(1)
			panic("Module Start must not run during inspection")
		},
	}
	if reverse {
		storage.Definition.Manifest.Provides = []module.Capability{
			"records.clock",
			module.CapabilityDatabase,
			"records.store",
		}
		storage.Definition.Migrations[0], storage.Definition.Migrations[1] = storage.Definition.Migrations[1], storage.Definition.Migrations[0]
		feature.Definition.Manifest.Requires = []module.Capability{"records.clock", "records.store"}
		feature.Definition.Actions[0], feature.Definition.Actions[1] = feature.Definition.Actions[1], feature.Definition.Actions[0]
		return appkit.Definition{Metadata: fixtureMetadata(), Modules: []module.Registration{feature, storage}}
	}
	return appkit.Definition{Metadata: fixtureMetadata(), Modules: []module.Registration{storage, feature}}
}

func fixtureAction(id string, preview bool, counters *inspectionCounters) module.ActionBinding {
	descriptor := action.Descriptor{
		ID:           id,
		Version:      "1.0.0",
		Title:        "Fixture " + id,
		InputSchema:  []byte(`{"required":["name"],"properties":{"name":{"type":"string"},"count":{"type":"integer"}},"type":"object","additionalProperties":false}`),
		OutputSchema: []byte(`{"properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false,"type":"object"}`),
		Permission:   id,
		Preview:      action.PreviewNone,
		AuditLevel:   action.AuditDetailed,
		Channels:     []action.Channel{action.ChannelMCP, action.ChannelCLI, action.ChannelHTTP},
	}
	if preview {
		descriptor.Preview = action.PreviewRequired
		descriptor.PreviewSchema = []byte(`{"required":["summary"],"type":"object","properties":{"summary":{"type":"string"}},"additionalProperties":false}`)
	}
	return module.ActionBinding{
		Descriptor: descriptor,
		NewHandler: func(context.Context, module.Resolver) (action.Handler, error) {
			counters.handlers.Add(1)
			panic("Action handler factory must not run during inspection")
		},
	}
}

func fixtureMetadata() appkit.Metadata {
	return appkit.Metadata{ID: "example-app", Name: "Example App", Version: "1.2.3"}
}

func writeFixtureProject(t *testing.T, manifest string) string {
	t.Helper()
	root := t.TempDir()
	if manifest == "" {
		manifest = validProjectManifest
	}
	if err := os.WriteFile(filepath.Join(root, ProjectManifestName), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write project manifest: %v", err)
	}
	return root
}

func loadFixtureProject(t *testing.T, root string) *Project {
	t.Helper()
	project, err := Load(root)
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	return project
}

func assertNoInspectionSideEffects(t *testing.T, counters *inspectionCounters) {
	t.Helper()
	if got := counters.starts.Load(); got != 0 {
		t.Fatalf("Start calls = %d, want 0", got)
	}
	if got := counters.handlers.Load(); got != 0 {
		t.Fatalf("handler factory calls = %d, want 0", got)
	}
	if got := counters.opens.Load(); got != 0 {
		t.Fatalf("migration Open calls = %d, want 0", got)
	}
}

func readFixtureFile(t *testing.T, root, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return data
}

func assertNoTemporaryArtifacts(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Name() != ProjectManifestName && len(entry.Name()) >= len(".modary-") && entry.Name()[:len(".modary-")] == ".modary-" {
			return fmt.Errorf("temporary artifact remains: %s", name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
