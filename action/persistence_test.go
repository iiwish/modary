package action

import (
	"strings"
	"testing"

	"github.com/iiwish/modary/authz"
)

func TestPersistenceTokensUseCanonicalPortableContracts(t *testing.T) {
	for _, hash := range []string{persistentDigestForTest('a'), persistentDigestForTest('f')} {
		if err := ValidatePlanHash(hash); err != nil {
			t.Fatalf("ValidatePlanHash(%q): %v", hash, err)
		}
	}
	for _, hash := range []string{"", "sha256:short", "sha256:" + strings.Repeat("A", 64)} {
		if err := ValidatePlanHash(hash); err == nil {
			t.Fatalf("ValidatePlanHash(%q) succeeded", hash)
		}
	}

	for _, snapshot := range []string{"", persistentDigestForTest('d')} {
		if err := ValidateSnapshotHash(snapshot); err != nil {
			t.Fatalf("ValidateSnapshotHash(%q): %v", snapshot, err)
		}
	}
	for _, snapshot := range []string{"opaque-version", "sha256:short", "sha256:" + strings.Repeat("A", 64)} {
		if err := ValidateSnapshotHash(snapshot); err == nil {
			t.Fatalf("ValidateSnapshotHash(%q) succeeded", snapshot)
		}
	}

	fingerprint := strings.Repeat("界", authz.MaxFingerprintRunes)
	if err := ValidateDecisionFingerprint(fingerprint); err != nil {
		t.Fatalf("maximum fingerprint rejected: %v", err)
	}
	for _, invalid := range []string{"", fingerprint + "界", "bad value", "bad\nvalue", string([]byte{0xff})} {
		if err := ValidateDecisionFingerprint(invalid); err == nil {
			t.Fatalf("ValidateDecisionFingerprint(%q) succeeded", invalid)
		}
	}
}

func persistentDigestForTest(character byte) string {
	return "sha256:" + strings.Repeat(string(character), 64)
}
