package sqlaudit

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/internal/databasecontrol"
	"golang.org/x/mod/semver"
)

// ErrContextRequired reports a nil audit persistence context.
var ErrContextRequired = errors.New("SQL Audit context is required")

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

const sqliteTimestampFormat = "2006-01-02T15:04:05.000000000Z07:00"

type hook struct{ control databasecontrol.Control }

// Record validates and persists one normalized audit event.
func (store *hook) Record(ctx context.Context, event audit.Event) error {
	if ctx == nil {
		return ErrContextRequired
	}
	if store == nil || store.control == nil {
		return fmt.Errorf("SQL Audit Hook is unavailable")
	}
	if err := validateRawEvent(event); err != nil {
		return err
	}
	event = audit.Normalize(event)
	if err := validateEvent(event); err != nil {
		return err
	}
	resources := []string{}
	var rows any
	if event.Impact != nil {
		resources = append(resources, event.Impact.Resources...)
		rows = event.Impact.Rows
	}
	resourcesJSON, err := json.Marshal(resources)
	if err != nil {
		return fmt.Errorf("encode audit resources: %w", err)
	}
	references := append([]audit.Reference(nil), event.ResultRefs...)
	if references == nil {
		references = []audit.Reference{}
	}
	referencesJSON, err := json.Marshal(references)
	if err != nil {
		return fmt.Errorf("encode audit references: %w", err)
	}
	executor, err := store.control.Executor(ctx)
	if err != nil {
		return fmt.Errorf("SQL Audit executor is unavailable: %w", err)
	}
	_, err = executor.ExecContext(ctx, `
		INSERT INTO modary_audit_event
		(request_id, actor_id, actor_type, channel, action_id, action_version,
		 contract_hash, scope_kind, scope_id, input_hash, plan_hash, decision,
		 audit_level, result_summary, impact_rows, impact_resources_json,
		 result_references_json, error_code, error_kind, reason, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.RequestID, event.ActorID, event.ActorType, event.Channel, event.ActionID,
		event.ActionVersion, event.ContractHash, event.Scope.Kind, event.Scope.ID,
		event.InputHash, event.PlanHash, event.Decision, event.AuditLevel,
		event.ResultSummary, rows, string(resourcesJSON), string(referencesJSON),
		event.ErrorCode, event.ErrorKind, event.Reason, formatTimestamp(event.StartedAt),
		formatTimestamp(event.FinishedAt))
	if err != nil {
		return fmt.Errorf("persist audit event: %w", err)
	}
	return nil
}

func validateEvent(event audit.Event) error {
	if err := validateRequired("request id", event.RequestID, audit.MaxRequestIDRunes); err != nil {
		return err
	}
	if err := validateRequired("action id", event.ActionID, audit.MaxActionIDRunes); err != nil {
		return err
	}
	if !validDecision(event.Decision) {
		return fmt.Errorf("audit decision %q is invalid", event.Decision)
	}
	if event.AuditLevel != "metadata" && event.AuditLevel != "detailed" {
		return fmt.Errorf("audit level %q is invalid", event.AuditLevel)
	}
	if event.AuditLevel == "metadata" && (event.Impact != nil || len(event.ResultRefs) != 0) {
		return fmt.Errorf("metadata audit event cannot contain impact or result references")
	}
	if successfulDecision(event.Decision) {
		if err := validateSuccessfulProvenance(event); err != nil {
			return err
		}
	} else if err := validateOptionalProvenance(event); err != nil {
		return err
	}
	if event.Scope.IsZero() {
		// Invalid or unauthenticated request attempts may be audited without a
		// usable scope; a partially populated scope is never accepted.
	} else if err := event.Scope.Validate(); err != nil {
		return fmt.Errorf("audit scope: %w", err)
	}
	if event.StartedAt.IsZero() || event.FinishedAt.IsZero() || event.FinishedAt.Before(event.StartedAt) {
		return fmt.Errorf("audit time range is invalid")
	}
	for _, value := range []time.Time{event.StartedAt, event.FinishedAt} {
		parsed, err := parseTimestamp(formatTimestamp(value))
		if err != nil || !parsed.Equal(value.UTC()) {
			return fmt.Errorf("audit timestamp is outside the canonical persistence range")
		}
	}
	if utf8.RuneCountInString(event.ResultSummary) > audit.MaxSummaryRunes || utf8.RuneCountInString(event.Reason) > audit.MaxReasonRunes {
		return fmt.Errorf("audit text exceeds persistence bounds")
	}
	if event.Impact != nil {
		if event.Impact.Rows < 0 || len(event.Impact.Resources) > audit.MaxResources {
			return fmt.Errorf("audit impact is invalid")
		}
		seen := make(map[string]struct{}, len(event.Impact.Resources))
		for _, resource := range event.Impact.Resources {
			if err := validateRequired("audit resource", resource, audit.MaxResourceRunes); err != nil {
				return err
			}
			if _, duplicate := seen[resource]; duplicate {
				return fmt.Errorf("audit resource %q is duplicated", resource)
			}
			seen[resource] = struct{}{}
		}
	}
	if len(event.ResultRefs) > audit.MaxReferences {
		return fmt.Errorf("audit references exceed persistence bounds")
	}
	seenReferences := make(map[string]struct{}, len(event.ResultRefs))
	for _, reference := range event.ResultRefs {
		if err := validateRequired("audit reference kind", reference.Kind, audit.MaxKindRunes); err != nil {
			return err
		}
		if err := validateRequired("audit reference id", reference.ID, audit.MaxIDRunes); err != nil {
			return err
		}
		key := reference.Kind + "\x00" + reference.ID
		if _, duplicate := seenReferences[key]; duplicate {
			return fmt.Errorf("audit reference %s/%s is duplicated", reference.Kind, reference.ID)
		}
		seenReferences[key] = struct{}{}
	}
	return nil
}

func validateRawEvent(event audit.Event) error {
	if !validDecision(event.Decision) {
		return fmt.Errorf("audit decision %q is invalid", event.Decision)
	}
	if event.AuditLevel != "metadata" && event.AuditLevel != "detailed" {
		return fmt.Errorf("audit level %q is invalid", event.AuditLevel)
	}
	if successfulDecision(event.Decision) {
		return validateSuccessfulProvenance(event)
	}
	return nil
}

func validDecision(decision string) bool {
	switch decision {
	case "allowed", "denied", "rejected", "failed", "idempotent_replay", "previewed":
		return true
	default:
		return false
	}
}

func successfulDecision(decision string) bool {
	return decision == "allowed" || decision == "idempotent_replay" || decision == "previewed"
}

func validateSuccessfulProvenance(event audit.Event) error {
	for _, field := range []struct {
		name  string
		value string
		limit int
	}{
		{name: "audit actor id", value: event.ActorID, limit: audit.MaxActorIDRunes},
		{name: "audit actor type", value: event.ActorType, limit: audit.MaxActorTypeRunes},
		{name: "audit channel", value: event.Channel, limit: audit.MaxChannelRunes},
	} {
		if err := validateRequired(field.name, field.value, field.limit); err != nil {
			return err
		}
	}
	if !action.ValidIdentifier(event.ActionID) {
		return fmt.Errorf("audit action id %q is invalid", event.ActionID)
	}
	if !validVersion(event.ActionVersion) {
		return fmt.Errorf("audit action version %q is invalid", event.ActionVersion)
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "contract hash", value: event.ContractHash},
		{name: "input hash", value: event.InputHash},
		{name: "plan hash", value: event.PlanHash},
	} {
		if !digestPattern.MatchString(field.value) {
			return fmt.Errorf("audit %s must be a lowercase SHA-256 digest", field.name)
		}
	}
	if err := event.Scope.Validate(); err != nil {
		return fmt.Errorf("audit scope: %w", err)
	}
	if event.ErrorCode != "" {
		return fmt.Errorf("successful audit event cannot contain an error code")
	}
	if event.ErrorKind != "" {
		return fmt.Errorf("successful audit event cannot contain an error kind")
	}
	if event.AuditLevel == "detailed" && event.Impact == nil {
		return fmt.Errorf("detailed successful audit event requires impact")
	}
	return nil
}

func validateOptionalProvenance(event audit.Event) error {
	for _, field := range []struct {
		name  string
		value string
		limit int
	}{
		{name: "audit actor id", value: event.ActorID, limit: audit.MaxActorIDRunes},
		{name: "audit actor type", value: event.ActorType, limit: audit.MaxActorTypeRunes},
		{name: "audit channel", value: event.Channel, limit: audit.MaxChannelRunes},
		{name: "audit error code", value: event.ErrorCode, limit: audit.MaxCodeRunes},
		{name: "audit error kind", value: event.ErrorKind, limit: audit.MaxKindRunes},
	} {
		if field.value != "" {
			if err := validateRequired(field.name, field.value, field.limit); err != nil {
				return err
			}
		}
	}
	if event.ErrorCode == "" || !action.ErrorKind(event.ErrorKind).Valid() {
		return fmt.Errorf("failed audit event must contain a valid error code and kind")
	}
	kind := action.ErrorKind(event.ErrorKind)
	if builtinKind, builtin := action.BuiltinErrorKind(event.ErrorCode); builtin {
		if kind != builtinKind {
			return fmt.Errorf("audit error code and kind do not match")
		}
	} else {
		if !action.ValidCustomErrorCode(event.ErrorCode) {
			return fmt.Errorf("audit error code is invalid")
		}
		// Action descriptors cannot declare a consumer-owned internal error. Keep
		// persisted events subject to the same invariant even when Hook is called
		// directly rather than by Runtime.
		if kind == action.ErrorKindInternal {
			return fmt.Errorf("consumer audit error cannot use the internal kind")
		}
	}
	switch event.Decision {
	case "denied":
		if kind != action.ErrorKindDenied {
			return fmt.Errorf("denied audit event must use the denied error kind")
		}
	case "rejected":
		if kind == action.ErrorKindDenied || kind == action.ErrorKindUnavailable || kind == action.ErrorKindInternal {
			return fmt.Errorf("rejected audit event has an invalid error kind")
		}
	case "failed":
		if kind != action.ErrorKindUnavailable && kind != action.ErrorKindInternal {
			return fmt.Errorf("failed audit event has an invalid error kind")
		}
	}
	if event.ActionVersion != "" && !validVersion(event.ActionVersion) {
		return fmt.Errorf("audit action version %q is invalid", event.ActionVersion)
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "contract hash", value: event.ContractHash},
		{name: "input hash", value: event.InputHash},
		{name: "plan hash", value: event.PlanHash},
	} {
		if field.value != "" && !digestPattern.MatchString(field.value) {
			return fmt.Errorf("audit %s must be a lowercase SHA-256 digest", field.name)
		}
	}
	return nil
}

func validVersion(value string) bool {
	core := value
	if index := strings.IndexAny(core, "-+"); index >= 0 {
		core = core[:index]
	}
	return len(value) <= audit.MaxVersionRunes && strings.Count(core, ".") == 2 && semver.IsValid("v"+value)
}

func validateRequired(name, value string, limit int) error {
	if !utf8.ValidString(value) || value == "" || utf8.RuneCountInString(value) > limit ||
		value != strings.TrimSpace(value) || containsControl(value) {
		return fmt.Errorf("%s is invalid", name)
	}
	return nil
}

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func (store *hook) load(ctx context.Context, eventID int64) (audit.Event, error) {
	if ctx == nil {
		return audit.Event{}, ErrContextRequired
	}
	var event audit.Event
	var started, finished string
	var rows sql.NullInt64
	var resourcesJSON, referencesJSON string
	executor, err := store.control.Executor(ctx)
	if err != nil {
		return audit.Event{}, fmt.Errorf("SQL Audit executor is unavailable: %w", err)
	}
	err = executor.QueryRowContext(ctx, `
		SELECT request_id, actor_id, actor_type, channel, action_id, action_version,
		       contract_hash, scope_kind, scope_id, input_hash, plan_hash, decision,
		       audit_level, result_summary, impact_rows, impact_resources_json,
		       result_references_json, error_code, error_kind, reason, started_at, finished_at
		FROM modary_audit_event WHERE event_id = ?`, eventID).Scan(
		&event.RequestID, &event.ActorID, &event.ActorType, &event.Channel,
		&event.ActionID, &event.ActionVersion, &event.ContractHash, &event.Scope.Kind,
		&event.Scope.ID, &event.InputHash, &event.PlanHash, &event.Decision,
		&event.AuditLevel, &event.ResultSummary, &rows, &resourcesJSON,
		&referencesJSON, &event.ErrorCode, &event.ErrorKind, &event.Reason, &started, &finished)
	if err != nil {
		return audit.Event{}, err
	}
	var resources []string
	if err := decodeStrictJSON([]byte(resourcesJSON), &resources); err != nil {
		return audit.Event{}, fmt.Errorf("decode audit resources: %w", err)
	}
	if rows.Valid {
		event.Impact = &audit.Impact{Rows: int(rows.Int64), Resources: resources}
	} else if len(resources) != 0 {
		return audit.Event{}, fmt.Errorf("decode audit impact: resources exist without row impact")
	}
	if err := decodeStrictJSON([]byte(referencesJSON), &event.ResultRefs); err != nil {
		return audit.Event{}, fmt.Errorf("decode audit references: %w", err)
	}
	event.StartedAt, err = parseTimestamp(started)
	if err != nil {
		return audit.Event{}, fmt.Errorf("decode audit start time: %w", err)
	}
	event.FinishedAt, err = parseTimestamp(finished)
	if err != nil {
		return audit.Event{}, fmt.Errorf("decode audit finish time: %w", err)
	}
	if err := validateEvent(event); err != nil {
		return audit.Event{}, fmt.Errorf("decode audit event: %w", err)
	}
	return event, nil
}

func formatTimestamp(value time.Time) string {
	return value.UTC().Format(sqliteTimestampFormat)
}

func parseTimestamp(value string) (time.Time, error) {
	parsed, err := time.Parse(sqliteTimestampFormat, value)
	if err != nil || value != formatTimestamp(parsed) {
		return time.Time{}, fmt.Errorf("timestamp is not canonical UTC")
	}
	return parsed, nil
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

var _ audit.Hook = (*hook)(nil)
