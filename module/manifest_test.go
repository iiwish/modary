package module

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCapabilityIsOpenAndRetainsStringWireValues(t *testing.T) {
	custom := Capability("records/read")
	manifest := Manifest{
		SchemaVersion: SchemaVersion,
		ID:            "counter",
		Version:       "0.1.0",
		Type:          ModuleTypeFeature,
		Requires:      []Capability{CapabilityDatabase, custom},
		Provides:      []Capability{CapabilityIdentity},
	}
	if err := ValidateManifest(manifest); err != nil {
		t.Fatalf("ValidateManifest() error = %v", err)
	}

	encodedJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		`"type":"feature"`,
		`"requires":["database","records/read"]`,
		`"provides":["identity"]`,
	} {
		if !strings.Contains(string(encodedJSON), fragment) {
			t.Fatalf("JSON Manifest = %s; missing %s", encodedJSON, fragment)
		}
	}

	encodedYAML, err := yaml.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"- database\n", "- records/read\n", "- identity\n"} {
		if !strings.Contains(string(encodedYAML), fragment) {
			t.Fatalf("YAML Manifest = %s; missing %q", encodedYAML, fragment)
		}
	}
}

func TestManifestUsesSemanticVersioningTwo(t *testing.T) {
	t.Parallel()

	base := validManifest("counter", "feature", nil, []string{"counter"})
	for _, version := range []string{"0.0.0", "1.2.3-alpha.1", "1.2.3+build.7", "1.2.3-rc.1+sha.abc"} {
		manifest := base
		manifest.Version = version
		if err := ValidateManifest(manifest); err != nil {
			t.Errorf("valid version %q rejected: %v", version, err)
		}
	}
	for _, version := range []string{"1.0", "v1.0.0", "1.0.0-01", "1.0.0-.", "1.0.0+"} {
		manifest := base
		manifest.Version = version
		if err := ValidateManifest(manifest); err == nil {
			t.Errorf("invalid version %q accepted", version)
		}
	}
}
