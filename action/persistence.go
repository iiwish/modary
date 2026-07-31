package action

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/modary/authz"
)

var planHashPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ValidatePlanHash validates a canonical Action plan SHA-256 digest.
func ValidatePlanHash(value string) error {
	if !planHashPattern.MatchString(value) {
		return fmt.Errorf("plan hash must be a lowercase SHA-256 digest")
	}
	return nil
}

// ValidateSnapshotHash validates an optional optimistic-concurrency snapshot.
// Non-empty values are canonical lowercase SHA-256 digests.
func ValidateSnapshotHash(value string) error {
	if value != "" && !planHashPattern.MatchString(value) {
		return fmt.Errorf("snapshot hash must be empty or a lowercase SHA-256 digest")
	}
	return nil
}

// ValidateDecisionFingerprint validates a required authorization policy
// fingerprint. Fingerprints are opaque, whitespace-free tokens bounded by the
// authz contract.
func ValidateDecisionFingerprint(value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("decision fingerprint must be valid UTF-8")
	}
	if value == "" {
		return fmt.Errorf("decision fingerprint is required")
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("decision fingerprint cannot contain surrounding whitespace")
	}
	if utf8.RuneCountInString(value) > authz.MaxFingerprintRunes {
		return fmt.Errorf("decision fingerprint cannot exceed %d characters", authz.MaxFingerprintRunes)
	}
	if strings.ContainsFunc(value, unicode.IsControl) {
		return fmt.Errorf("decision fingerprint cannot contain control characters")
	}
	if strings.ContainsFunc(value, unicode.IsSpace) {
		return fmt.Errorf("decision fingerprint cannot contain whitespace")
	}
	return nil
}
