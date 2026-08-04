package processkit_test

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/modary/processkit"
)

func TestBuildInfoHasExplicitBoundedDefaultsAndStructuredValue(t *testing.T) {
	defaults, err := processkit.NormalizeBuildInfo(processkit.BuildInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Version != "dev" || defaults.Revision != "unknown" || defaults.Created != "unknown" {
		t.Fatalf("defaults = %#v", defaults)
	}
	created := time.Date(2026, time.August, 4, 1, 2, 3, 0, time.UTC).Format(time.RFC3339)
	info, err := processkit.NormalizeBuildInfo(processkit.BuildInfo{Version: "v0.3.0-alpha.1", Revision: "abc123", Created: created})
	if err != nil {
		t.Fatal(err)
	}
	value := info.LogValue().Resolve()
	if value.Kind() != slog.KindGroup || len(value.Group()) != 3 {
		t.Fatalf("LogValue() = %#v", value)
	}
	for _, invalid := range []processkit.BuildInfo{
		{Version: strings.Repeat("v", 129)},
		{Revision: "secret\nvalue"},
		{Created: "2026-08-04"},
		{Created: "2026-08-04T01:02:03+00:00"},
	} {
		if _, err := processkit.NormalizeBuildInfo(invalid); err == nil {
			t.Fatalf("NormalizeBuildInfo(%#v) succeeded", invalid)
		}
	}
}
