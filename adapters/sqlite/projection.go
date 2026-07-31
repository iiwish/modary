package sqlite

import (
	"database/sql"
	"fmt"
	"unicode/utf8"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
)

const (
	maxStoredHashBytes           = audit.MaxHashRunes
	maxStoredActionIDBytes       = audit.MaxActionIDRunes
	maxStoredVersionBytes        = audit.MaxVersionRunes
	maxStoredActorIDBytes        = audit.MaxActorIDRunes * utf8.UTFMax
	maxStoredActorTypeBytes      = audit.MaxActorTypeRunes * utf8.UTFMax
	maxStoredChannelBytes        = audit.MaxChannelRunes * utf8.UTFMax
	maxStoredScopeKindBytes      = audit.MaxScopeKindRunes * utf8.UTFMax
	maxStoredScopeIDBytes        = audit.MaxScopeIDRunes * utf8.UTFMax
	maxStoredFingerprintBytes    = authz.MaxFingerprintRunes * utf8.UTFMax
	maxStoredIdempotencyKeyBytes = 256
	maxStoredStatusBytes         = len("completed")
	maxStoredSummaryBytes        = audit.MaxSummaryRunes * utf8.UTFMax
	maxStoredTimestampBytes      = 30
)

func storedProjectionArguments() []any {
	return []any{
		sql.Named("hash_bytes", maxStoredHashBytes),
		sql.Named("action_id_bytes", maxStoredActionIDBytes),
		sql.Named("version_bytes", maxStoredVersionBytes),
		sql.Named("actor_id_bytes", maxStoredActorIDBytes),
		sql.Named("actor_type_bytes", maxStoredActorTypeBytes),
		sql.Named("channel_bytes", maxStoredChannelBytes),
		sql.Named("scope_kind_bytes", maxStoredScopeKindBytes),
		sql.Named("scope_id_bytes", maxStoredScopeIDBytes),
		sql.Named("fingerprint_bytes", maxStoredFingerprintBytes),
		sql.Named("idempotency_key_bytes", maxStoredIdempotencyKeyBytes),
		sql.Named("status_bytes", maxStoredStatusBytes),
		sql.Named("summary_bytes", maxStoredSummaryBytes),
		sql.Named("timestamp_bytes", maxStoredTimestampBytes),
		sql.Named("json_bytes", action.MaxJSONDocumentBytes),
	}
}

func appendStoredProjectionArguments(arguments []any, values ...sql.NamedArg) []any {
	arguments = append(arguments, storedProjectionArguments()...)
	for _, value := range values {
		arguments = append(arguments, value)
	}
	return arguments
}

func requireProjectedText(value sql.NullString, field string) (string, error) {
	if !value.Valid {
		return "", fmt.Errorf("stored %s is absent, has the wrong type, or exceeds its resource limit", field)
	}
	return value.String, nil
}

func requireProjectedInteger(value sql.NullInt64, field string) (int64, error) {
	if !value.Valid {
		return 0, fmt.Errorf("stored %s is absent or has the wrong type", field)
	}
	return value.Int64, nil
}
