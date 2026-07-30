package module

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const SchemaVersion = "modary.module/v1alpha1"

type Manifest struct {
	SchemaVersion string            `yaml:"schemaVersion" json:"schemaVersion"`
	ID            string            `yaml:"id" json:"id"`
	Version       string            `yaml:"version" json:"version"`
	Type          string            `yaml:"type" json:"type"`
	Requires      []string          `yaml:"requires,omitempty" json:"requires,omitempty"`
	Provides      []string          `yaml:"provides,omitempty" json:"provides,omitempty"`
	Actions       []string          `yaml:"actions,omitempty" json:"actions,omitempty"`
	Migrations    map[string]string `yaml:"migrations,omitempty" json:"migrations,omitempty"`
	UI            UIContribution    `yaml:"ui,omitempty" json:"ui,omitempty"`
}

type UIContribution struct {
	Routes []UIRoute `yaml:"routes,omitempty" json:"routes,omitempty"`
}

type UIRoute struct {
	ID    string `yaml:"id" json:"id"`
	Path  string `yaml:"path" json:"path"`
	Entry string `yaml:"entry,omitempty" json:"entry,omitempty"`
}

type AppManifest struct {
	App struct {
		ID string `yaml:"id" json:"id"`
	} `yaml:"app" json:"app"`
	Modules []string `yaml:"modules" json:"modules"`
}

func ParseManifest(data []byte) (Manifest, error) {
	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse module manifest: %w", err)
	}
	if err := ValidateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func MustParseManifest(data []byte) Manifest {
	manifest, err := ParseManifest(data)
	if err != nil {
		panic(err)
	}
	return manifest
}

func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read module manifest %s: %w", path, err)
	}
	return ParseManifest(data)
}

func LoadAppManifest(path string) (AppManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AppManifest{}, fmt.Errorf("read app manifest %s: %w", path, err)
	}
	var manifest AppManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return AppManifest{}, fmt.Errorf("parse app manifest: %w", err)
	}
	if manifest.App.ID == "" {
		return AppManifest{}, fmt.Errorf("app.id is required")
	}
	if len(manifest.Modules) == 0 {
		return AppManifest{}, fmt.Errorf("at least one module is required")
	}
	return manifest, nil
}

func ValidateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != SchemaVersion {
		return fmt.Errorf("module %q uses unsupported schemaVersion %q", manifest.ID, manifest.SchemaVersion)
	}
	if manifest.ID == "" {
		return fmt.Errorf("module id is required")
	}
	if manifest.Version == "" {
		return fmt.Errorf("module %s version is required", manifest.ID)
	}
	if manifest.Type != "feature" && manifest.Type != "adapter" {
		return fmt.Errorf("module %s has invalid type %q", manifest.ID, manifest.Type)
	}
	for _, route := range manifest.UI.Routes {
		if strings.TrimSpace(route.ID) == "" || strings.TrimSpace(route.Path) == "" || strings.TrimSpace(route.Entry) == "" {
			return fmt.Errorf("module %s UI routes must declare id, path, and entry", manifest.ID)
		}
	}
	return nil
}
