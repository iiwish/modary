package module

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

// SchemaVersion is the Module manifest schema accepted by this package.
const SchemaVersion = "modary.module/v1alpha2"

// ModuleType classifies a Module's role in the application graph.
type ModuleType string

const (
	// ModuleTypeFeature identifies a Module that contributes product behavior.
	ModuleTypeFeature ModuleType = "feature"
	// ModuleTypeAdapter identifies a Module that provides infrastructure.
	ModuleTypeAdapter ModuleType = "adapter"
)

// Capability identifies an open Module dependency contract. Framework
// capabilities use the constants below; consumers may declare additional
// validated values for application-specific services.
type Capability string

const (
	// CapabilityDatabase identifies the canonical database service.
	CapabilityDatabase Capability = "database"
	// CapabilityIdentity identifies canonical identity services.
	CapabilityIdentity Capability = "identity"
	// CapabilityAuthorization identifies the canonical authorization service.
	CapabilityAuthorization Capability = "authorization"
	// CapabilityAudit identifies the canonical audit service.
	CapabilityAudit Capability = "audit"
	// CapabilityTasks identifies the canonical durable task service.
	CapabilityTasks Capability = "tasks"
)

var (
	moduleIDPattern   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	capabilityPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}(?:[./][a-z][a-z0-9-]{0,62})*$`)
)

// Manifest is the stable, serializable identity and capability contract for a
// Module. Definition adds the Module's Actions and migration sources.
type Manifest struct {
	SchemaVersion string       `yaml:"schemaVersion" json:"schemaVersion"`
	ID            string       `yaml:"id" json:"id"`
	Version       string       `yaml:"version" json:"version"`
	Type          ModuleType   `yaml:"type" json:"type"`
	Requires      []Capability `yaml:"requires,omitempty" json:"requires,omitempty"`
	Provides      []Capability `yaml:"provides,omitempty" json:"provides,omitempty"`
}

// ValidateManifest checks identity, version, type, and capability invariants.
func ValidateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != SchemaVersion {
		return fmt.Errorf("module %q uses unsupported schemaVersion %q", manifest.ID, manifest.SchemaVersion)
	}
	if !moduleIDPattern.MatchString(manifest.ID) {
		return fmt.Errorf("module id %q is invalid", manifest.ID)
	}
	coreVersion := manifest.Version
	if index := strings.IndexAny(coreVersion, "-+"); index >= 0 {
		coreVersion = coreVersion[:index]
	}
	if strings.Count(coreVersion, ".") != 2 || !semver.IsValid("v"+manifest.Version) {
		return fmt.Errorf("module %s version %q is not valid Semantic Versioning 2.0.0", manifest.ID, manifest.Version)
	}
	if manifest.Type != ModuleTypeFeature && manifest.Type != ModuleTypeAdapter {
		return fmt.Errorf("module %s has invalid type %q", manifest.ID, manifest.Type)
	}
	if err := validateCapabilities(manifest.ID, "requires", manifest.Requires); err != nil {
		return err
	}
	return validateCapabilities(manifest.ID, "provides", manifest.Provides)
}

func validateCapabilities(moduleID, field string, capabilities []Capability) error {
	seen := make(map[Capability]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if len(capability) > 127 || !capabilityPattern.MatchString(string(capability)) {
			return fmt.Errorf("module %s %s invalid capability %q", moduleID, field, capability)
		}
		if _, exists := seen[capability]; exists {
			return fmt.Errorf("module %s %s capability %q more than once", moduleID, field, capability)
		}
		seen[capability] = struct{}{}
	}
	return nil
}
