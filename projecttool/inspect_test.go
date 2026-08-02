package projecttool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/module"
)

func TestInspectIsPureDeterministicAndDefensive(t *testing.T) {
	firstCounters := &inspectionCounters{}
	first, err := Inspect(fixtureDefinition(firstCounters, false))
	if err != nil {
		t.Fatalf("Inspect first Definition: %v", err)
	}
	secondCounters := &inspectionCounters{}
	second, err := Inspect(fixtureDefinition(secondCounters, true))
	if err != nil {
		t.Fatalf("Inspect shuffled Definition: %v", err)
	}
	assertNoInspectionSideEffects(t, firstCounters)
	assertNoInspectionSideEffects(t, secondCounters)

	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("shuffled Definition changed snapshot\nfirst:  %s\nsecond: %s", firstJSON, secondJSON)
	}
	if got, want := []string{first.Modules[0].ID, first.Modules[1].ID}, []string{"records", "storage"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Module order = %v, want %v", got, want)
	}
	if got, want := first.Modules[0].Requires, []module.Capability{"records.clock", "records.store"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sorted requirements = %v, want %v", got, want)
	}
	if got, want := first.Modules[1].Migrations, []string{"archive", "postgres"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sorted migrations = %v, want %v", got, want)
	}
	if got, want := []string{first.Actions[0].Descriptor.ID, first.Actions[1].Descriptor.ID}, []string{"records.archive", "records.create"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Action order = %v, want %v", got, want)
	}
	for _, entry := range first.Actions {
		if entry.ModuleID != "records" {
			t.Fatalf("Action %s owner = %q", entry.Descriptor.ID, entry.ModuleID)
		}
		if got, want := entry.Descriptor.Channels, []action.Channel{action.ChannelCLI, action.ChannelHTTP, action.ChannelMCP}; !reflect.DeepEqual(got, want) {
			t.Fatalf("Action %s channels = %v, want %v", entry.Descriptor.ID, got, want)
		}
		if !json.Valid(entry.Descriptor.InputSchema) || len(entry.ContractHash) == 0 {
			t.Fatalf("Action %s was not compiled: %#v", entry.Descriptor.ID, entry)
		}
	}
	if got := first.Graph.Provides["records.store"]; got != "storage" {
		t.Fatalf("records.store provider = %q", got)
	}
	if got, want := len(first.Graph.Edges), 2; got != want {
		t.Fatalf("graph edges = %d, want %d", got, want)
	}
	if got := string(first.Actions[0].Descriptor.InputSchema); got != `{"additionalProperties":false,"properties":{"count":{"type":"integer"},"name":{"type":"string"}},"required":["name"],"type":"object"}` {
		t.Fatalf("canonical input schema = %s", got)
	}

	first.Modules[0].Requires[0] = "forged"
	first.Graph.Modules[0] = "forged"
	first.Graph.Provides["forged"] = "forged"
	first.Actions[0].Descriptor.InputSchema[0] = '!'
	fresh, err := Inspect(fixtureDefinition(&inspectionCounters{}, false))
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Modules[0].Requires[0] == "forged" || fresh.Graph.Modules[0] == "forged" || fresh.Graph.Provides["forged"] != "" || fresh.Actions[0].Descriptor.InputSchema[0] == '!' {
		t.Fatal("Snapshot mutation leaked into a later inspection")
	}
}

func TestInspectValidatesEveryStaticContractWithoutCallbacks(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *inspectionCounters) []module.Registration
	}{
		{
			name: "invalid Module manifest",
			mutate: func(t *testing.T, counters *inspectionCounters) []module.Registration {
				definition := fixtureDefinition(counters, false)
				definition.Modules[0].Definition.Manifest.Version = "1.0"
				return definition.Modules
			},
		},
		{
			name: "missing capability",
			mutate: func(t *testing.T, counters *inspectionCounters) []module.Registration {
				definition := fixtureDefinition(counters, false)
				definition.Modules[1].Definition.Manifest.Requires = append(definition.Modules[1].Definition.Manifest.Requires, "missing.service")
				return definition.Modules
			},
		},
		{
			name: "duplicate capability owner",
			mutate: func(t *testing.T, counters *inspectionCounters) []module.Registration {
				definition := fixtureDefinition(counters, false)
				definition.Modules = append(definition.Modules, module.Register(module.Manifest{
					SchemaVersion: module.SchemaVersion,
					ID:            "other-storage",
					Version:       "1.0.0",
					Type:          module.ModuleTypeAdapter,
					Provides:      []module.Capability{"records.store"},
				}, nil))
				return definition.Modules
			},
		},
		{
			name: "invalid migration declaration",
			mutate: func(t *testing.T, counters *inspectionCounters) []module.Registration {
				definition := fixtureDefinition(counters, false)
				definition.Modules[0].Definition.Migrations[0].Driver = "INVALID"
				return definition.Modules
			},
		},
		{
			name: "nil migration files",
			mutate: func(t *testing.T, counters *inspectionCounters) []module.Registration {
				definition := fixtureDefinition(counters, false)
				definition.Modules[0].Definition.Migrations[0].Files = (*failOnOpenFS)(nil)
				return definition.Modules
			},
		},
		{
			name: "nil handler factory",
			mutate: func(t *testing.T, counters *inspectionCounters) []module.Registration {
				definition := fixtureDefinition(counters, false)
				definition.Modules[1].Definition.Actions[0].NewHandler = nil
				return definition.Modules
			},
		},
		{
			name: "invalid Action contract",
			mutate: func(t *testing.T, counters *inspectionCounters) []module.Registration {
				definition := fixtureDefinition(counters, false)
				definition.Modules[1].Definition.Actions[0].Descriptor.InputSchema = []byte(`{"type":7}`)
				return definition.Modules
			},
		},
		{
			name: "duplicate Action ownership",
			mutate: func(t *testing.T, counters *inspectionCounters) []module.Registration {
				definition := fixtureDefinition(counters, false)
				duplicate := fixtureAction("records.create", false, counters)
				definition.Modules[0].Definition.Actions = append(definition.Modules[0].Definition.Actions, duplicate)
				return definition.Modules
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			counters := &inspectionCounters{}
			definition := fixtureDefinition(counters, false)
			definition.Modules = test.mutate(t, counters)
			if _, err := Inspect(definition); err == nil {
				t.Fatal("Inspect succeeded")
			}
			assertNoInspectionSideEffects(t, counters)
		})
	}
}

func TestInspectRejectsInvalidApplicationMetadataAndEmptyComposition(t *testing.T) {
	counters := &inspectionCounters{}
	definition := fixtureDefinition(counters, false)
	definition.Metadata.ID = "INVALID"
	if _, err := Inspect(definition); err == nil {
		t.Fatal("Inspect accepted invalid metadata")
	}
	assertNoInspectionSideEffects(t, counters)

	definition = fixtureDefinition(counters, false)
	definition.Modules = nil
	if _, err := Inspect(definition); err == nil {
		t.Fatal("Inspect accepted an empty composition")
	}
	assertNoInspectionSideEffects(t, counters)
}

func TestProjectVerifyRequiresExactManifestIdentityWithoutWriting(t *testing.T) {
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	counters := &inspectionCounters{}
	definition := fixtureDefinition(counters, false)
	definition.Metadata = appkit.Metadata{ID: "other-app", Name: "Other App", Version: "1.0.0"}
	before := snapshotTree(t, root)
	if _, err := project.Verify(definition); err == nil {
		t.Fatal("Verify accepted mismatched application identity")
	}
	if after := snapshotTree(t, root); !reflect.DeepEqual(after, before) {
		t.Fatal("Verify wrote files")
	}
	assertNoInspectionSideEffects(t, counters)
}

func TestInspectErrorsAreDeterministicAcrossCompositionOrder(t *testing.T) {
	first := fixtureDefinition(&inspectionCounters{}, false)
	first.Modules[1].Definition.Manifest.Requires = append(first.Modules[1].Definition.Manifest.Requires, "missing.service")
	second := fixtureDefinition(&inspectionCounters{}, true)
	second.Modules[0].Definition.Manifest.Requires = append(second.Modules[0].Definition.Manifest.Requires, "missing.service")
	_, firstErr := Inspect(first)
	_, secondErr := Inspect(second)
	if firstErr == nil || secondErr == nil {
		t.Fatalf("Inspect errors = %v / %v", firstErr, secondErr)
	}
	if firstErr.Error() != secondErr.Error() {
		t.Fatalf("shuffled errors differ:\n%s\n%s", firstErr, secondErr)
	}
}

func TestInspectDuplicateIdentityPreScanIsDeterministicBeforeInternalValidation(t *testing.T) {
	tests := []struct {
		name      string
		want      string
		duplicate func(appkit.Definition) (appkit.Definition, appkit.Definition)
	}{
		{
			name: "Module identity",
			want: `duplicate Module identity "storage"`,
			duplicate: func(first appkit.Definition) (appkit.Definition, appkit.Definition) {
				duplicate := first.Modules[0]
				duplicate.Definition.Manifest.Version = "invalid-internal-version"
				first.Modules = append(first.Modules, duplicate)
				second := first
				second.Modules = []module.Registration{first.Modules[2], first.Modules[1], first.Modules[0]}
				return first, second
			},
		},
		{
			name: "Action identity",
			want: `duplicate Action identity "records.create" owned by Module "records"`,
			duplicate: func(first appkit.Definition) (appkit.Definition, appkit.Definition) {
				registration := &first.Modules[1]
				duplicate := registration.Definition.Actions[0]
				duplicate.Descriptor.InputSchema = []byte(`{"type":7}`)
				registration.Definition.Actions = append(registration.Definition.Actions, duplicate)
				second := first
				second.Modules = append([]module.Registration(nil), first.Modules...)
				second.Modules[1].Definition.Actions = append([]module.ActionBinding(nil), registration.Definition.Actions...)
				actions := second.Modules[1].Definition.Actions
				actions[0], actions[len(actions)-1] = actions[len(actions)-1], actions[0]
				return first, second
			},
		},
		{
			name: "migration identity",
			want: `duplicate migration identity "postgres" owned by Module "storage"`,
			duplicate: func(first appkit.Definition) (appkit.Definition, appkit.Definition) {
				registration := &first.Modules[0]
				duplicate := registration.Definition.Migrations[0]
				duplicate.Files = nil
				registration.Definition.Migrations = append(registration.Definition.Migrations, duplicate)
				second := first
				second.Modules = append([]module.Registration(nil), first.Modules...)
				second.Modules[0].Definition.Migrations = append([]module.MigrationSource(nil), registration.Definition.Migrations...)
				migrations := second.Modules[0].Definition.Migrations
				migrations[0], migrations[len(migrations)-1] = migrations[len(migrations)-1], migrations[0]
				return first, second
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first, second := test.duplicate(fixtureDefinition(&inspectionCounters{}, false))
			_, firstErr := Inspect(first)
			_, secondErr := Inspect(second)
			if firstErr == nil || secondErr == nil {
				t.Fatalf("errors = %v / %v", firstErr, secondErr)
			}
			if firstErr.Error() != secondErr.Error() || firstErr.Error() != test.want {
				t.Fatalf("errors are not canonical:\nfirst:  %s\nsecond: %s\nwant:   %s", firstErr, secondErr, test.want)
			}
			if !strings.HasPrefix(firstErr.Error(), "duplicate") {
				t.Fatalf("internal validation won over duplicate pre-scan: %v", firstErr)
			}
		})
	}
}

func TestCanonicalJSONRejectsMultipleValues(t *testing.T) {
	if _, err := canonicalJSON([]byte(`{} {}`)); err == nil {
		t.Fatal("canonicalJSON accepted multiple values")
	}
}
