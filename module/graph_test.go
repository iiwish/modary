package module

import (
	"reflect"
	"strings"
	"testing"
)

func TestVerifyProducesCanonicalGraphAcrossInputOrder(t *testing.T) {
	provider := validManifest("provider", "adapter", nil, []string{"storage.read", "storage.write"})
	consumer := validManifest("consumer", "feature", []string{"storage.read", "storage.write"}, []string{"feature"})
	first, err := Verify([]Manifest{provider, consumer})
	if err != nil {
		t.Fatal(err)
	}
	consumer.Requires = []Capability{"storage.write", "storage.read"}
	provider.Provides = []Capability{"storage.write", "storage.read"}
	second, err := Verify([]Manifest{consumer, provider})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("semantically identical inputs produced different graphs:\nfirst:  %#v\nsecond: %#v", first, second)
	}
}

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

func validManifest(id string, kind ModuleType, requires, provides []string) Manifest {
	return Manifest{
		SchemaVersion: SchemaVersion,
		ID:            id,
		Version:       "0.1.0",
		Type:          kind,
		Requires:      capabilities(requires),
		Provides:      capabilities(provides),
	}
}

func capabilities(values []string) []Capability {
	if values == nil {
		return nil
	}
	result := make([]Capability, len(values))
	for index, value := range values {
		result[index] = Capability(value)
	}
	return result
}
