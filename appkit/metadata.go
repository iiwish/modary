package appkit

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/mod/semver"
)

const (
	maxMetadataNameRunes    = 160
	maxMetadataVersionRunes = 128
)

var metadataIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// Metadata is the consumer-owned identity reported by application transports.
type Metadata struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ValidateMetadata validates the application identity contract without
// starting Modules or performing any other side effect.
func ValidateMetadata(metadata Metadata) error {
	return validateMetadata(metadata)
}

func validateMetadata(metadata Metadata) error {
	if !metadataIDPattern.MatchString(metadata.ID) {
		return fmt.Errorf("application metadata id %q must match %s", metadata.ID, metadataIDPattern.String())
	}
	if err := validateMetadataText("name", metadata.Name, maxMetadataNameRunes); err != nil {
		return err
	}
	if err := validateMetadataText("version", metadata.Version, maxMetadataVersionRunes); err != nil {
		return err
	}
	coreVersion := metadata.Version
	if index := strings.IndexAny(coreVersion, "-+"); index >= 0 {
		coreVersion = coreVersion[:index]
	}
	if strings.Count(coreVersion, ".") != 2 || !semver.IsValid("v"+metadata.Version) {
		return fmt.Errorf("application metadata version %q is not valid Semantic Versioning 2.0.0", metadata.Version)
	}
	return nil
}

func validateMetadataText(field, value string, limit int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("application metadata %s must be valid UTF-8", field)
	}
	if value == "" {
		return fmt.Errorf("application metadata %s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("application metadata %s cannot contain surrounding whitespace", field)
	}
	if utf8.RuneCountInString(value) > limit {
		return fmt.Errorf("application metadata %s cannot exceed %d characters", field, limit)
	}
	if strings.ContainsFunc(value, unicode.IsControl) {
		return fmt.Errorf("application metadata %s cannot contain control characters", field)
	}
	return nil
}
