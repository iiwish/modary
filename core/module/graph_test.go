package module

import (
	"strings"
	"testing"
)

func TestVerifyOrdersProvidersBeforeConsumers(t *testing.T) {
	graph, err := Verify([]Manifest{
		validManifest("feature", "feature", []string{"database"}, []string{"feature"}),
		validManifest("database", "adapter", nil, []string{"database"}),
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if got := strings.Join(graph.Order, ","); got != "database,feature" {
		t.Fatalf("order = %s", got)
	}
}

func TestVerifyRejectsMissingCapability(t *testing.T) {
	_, err := Verify([]Manifest{validManifest("feature", "feature", []string{"database"}, nil)})
	if err == nil || !strings.Contains(err.Error(), "missing capability") {
		t.Fatalf("error = %v", err)
	}
}

func TestVerifyRejectsDuplicateProvider(t *testing.T) {
	_, err := Verify([]Manifest{
		validManifest("a", "adapter", nil, []string{"database"}),
		validManifest("b", "adapter", nil, []string{"database"}),
	})
	if err == nil || !strings.Contains(err.Error(), "multiple providers") {
		t.Fatalf("error = %v", err)
	}
}

func TestVerifyReportsFullCycle(t *testing.T) {
	_, err := Verify([]Manifest{
		validManifest("a", "feature", []string{"b"}, []string{"a"}),
		validManifest("b", "feature", []string{"a"}, []string{"b"}),
	})
	if err == nil || !strings.Contains(err.Error(), "a -> b -> a") {
		t.Fatalf("error = %v", err)
	}
}

func TestVerifyRejectsDuplicateActionAndRoute(t *testing.T) {
	a := validManifest("a", "feature", nil, []string{"a"})
	b := validManifest("b", "feature", nil, []string{"b"})
	a.Actions = []string{"thing.create"}
	b.Actions = []string{"thing.create"}
	_, err := Verify([]Manifest{a, b})
	if err == nil || !strings.Contains(err.Error(), "duplicate action") {
		t.Fatalf("error = %v", err)
	}
}

func validManifest(id, kind string, requires, provides []string) Manifest {
	return Manifest{
		SchemaVersion: SchemaVersion,
		ID:            id,
		Version:       "0.1.0",
		Type:          kind,
		Requires:      requires,
		Provides:      provides,
	}
}
