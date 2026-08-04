package processkit

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// BuildInfo is immutable process identity injected by the consumer build. It
// deliberately excludes host, user, and other high-cardinality dimensions.
type BuildInfo struct {
	Version  string
	Revision string
	Created  string
}

// NormalizeBuildInfo validates bounded identity and supplies explicit local
// development defaults.
func NormalizeBuildInfo(info BuildInfo) (BuildInfo, error) {
	if info.Version == "" {
		info.Version = "dev"
	}
	if info.Revision == "" {
		info.Revision = "unknown"
	}
	if info.Created == "" {
		info.Created = "unknown"
	}
	for name, value := range map[string]string{
		"version": info.Version, "revision": info.Revision, "created": info.Created,
	} {
		if !validBuildValue(value, 128) {
			return BuildInfo{}, fmt.Errorf("process build %s is invalid", name)
		}
	}
	if info.Created != "unknown" {
		parsed, err := time.Parse(time.RFC3339, info.Created)
		if err != nil || parsed.Format(time.RFC3339) != info.Created {
			return BuildInfo{}, fmt.Errorf("process build created must be canonical RFC3339 or unknown")
		}
	}
	return info, nil
}

// LogValue groups build identity in structured diagnostics.
func (info BuildInfo) LogValue() slog.Value {
	normalized, err := NormalizeBuildInfo(info)
	if err != nil {
		return slog.GroupValue(slog.String("version", "invalid"), slog.String("revision", "invalid"), slog.String("created", "invalid"))
	}
	return slog.GroupValue(
		slog.String("version", normalized.Version),
		slog.String("revision", normalized.Revision),
		slog.String("created", normalized.Created),
	)
}

func validBuildValue(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && utf8.ValidString(value) && strings.TrimSpace(value) == value && !strings.ContainsFunc(value, unicode.IsControl)
}
