package projecttool_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/projecttool"
)

func TestPublicConsumerProjectAPI(t *testing.T) {
	if got, want := reflect.TypeOf((*projecttool.DefinitionProvider)(nil)).Elem(),
		reflect.TypeOf((*appkit.DefinitionProvider)(nil)).Elem(); got != want {
		t.Fatalf("DefinitionProvider is not the appkit alias: %s", got)
	}

	root := t.TempDir()
	manifest := `application:
  id: public-consumer
  name: Public Consumer
  version: 0.1.0
outputs:
  graph: generated/graph.json
  actions: generated/actions.json
build:
  package: ./cmd/public-consumer
  output: dist/public-consumer
`
	if err := os.WriteFile(filepath.Join(root, projecttool.ProjectManifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	var starts atomic.Int64
	definition := appkit.Definition{
		Metadata: appkit.Metadata{ID: "public-consumer", Name: "Public Consumer", Version: "0.1.0"},
		Modules: []module.Registration{module.Register(module.Manifest{
			SchemaVersion: module.SchemaVersion,
			ID:            "public-feature",
			Version:       "1.0.0",
			Type:          module.ModuleTypeFeature,
		}, func(context.Context, module.Scope) error {
			starts.Add(1)
			panic("public inspection must not start Modules")
		})},
	}

	project, err := projecttool.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := project.Verify(definition); err != nil {
		t.Fatal(err)
	}
	if _, err := project.Generate(definition); err != nil {
		t.Fatal(err)
	}
	if drift, err := project.Check(definition); err != nil || len(drift) != 0 {
		t.Fatalf("Check = %#v, %v", drift, err)
	}
	if starts.Load() != 0 {
		t.Fatalf("Start calls = %d", starts.Load())
	}

	var graph projecttool.GraphDocument
	graphData, err := os.ReadFile(filepath.Join(root, "generated", "graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(graphData, &graph); err != nil {
		t.Fatal(err)
	}
	if graph.SchemaVersion != projecttool.GraphSchemaVersion || len(graph.Modules) != 1 {
		t.Fatalf("graph = %#v", graph)
	}
	var catalog projecttool.CatalogDocument
	catalogData, err := os.ReadFile(filepath.Join(root, "generated", "actions.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(catalogData, &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.SchemaVersion != projecttool.CatalogSchemaVersion || catalog.Actions == nil {
		t.Fatalf("catalog = %#v", catalog)
	}
}
