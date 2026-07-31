package audit

import (
	"context"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/modary/scope"
)

// Persistence limits bound every user- or consumer-controlled audit field.
const (
	MaxSummaryRunes   = 512
	MaxReasonRunes    = 2048
	MaxResources      = 32
	MaxReferences     = 32
	MaxResourceRunes  = 256
	MaxKindRunes      = 80
	MaxIDRunes        = 256
	MaxRequestIDRunes = 128
	MaxActorIDRunes   = 256
	MaxActorTypeRunes = 64
	MaxChannelRunes   = 64
	MaxActionIDRunes  = 127
	MaxVersionRunes   = 128
	MaxHashRunes      = 71
	MaxScopeKindRunes = 64
	MaxScopeIDRunes   = 256
	MaxCodeRunes      = 64
)

// Impact is the bounded mutation footprint attached to a detailed event.
type Impact struct {
	Rows      int      `json:"rows,omitempty"`
	Resources []string `json:"resources,omitempty"`
}

// Reference identifies a durable result without embedding business data.
type Reference struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// Event is one normalized record of a governed Action decision or outcome.
// ErrorCode and ErrorKind are both present only for denied, rejected, or failed
// outcomes; adapters validate their semantic pairing before persistence.
type Event struct {
	RequestID     string          `json:"request_id"`
	ActorID       string          `json:"actor_id,omitempty"`
	ActorType     string          `json:"actor_type,omitempty"`
	Channel       string          `json:"channel,omitempty"`
	ActionID      string          `json:"action_id"`
	ActionVersion string          `json:"action_version,omitempty"`
	ContractHash  string          `json:"contract_hash,omitempty"`
	Scope         scope.Execution `json:"scope"`
	InputHash     string          `json:"input_hash,omitempty"`
	PlanHash      string          `json:"plan_hash,omitempty"`
	Decision      string          `json:"decision"`
	AuditLevel    string          `json:"audit_level"`
	ResultSummary string          `json:"result_summary,omitempty"`
	Impact        *Impact         `json:"impact,omitempty"`
	ResultRefs    []Reference     `json:"result_refs,omitempty"`
	ErrorCode     string          `json:"error_code,omitempty"`
	ErrorKind     string          `json:"error_kind,omitempty"`
	Reason        string          `json:"reason,omitempty"`
	StartedAt     time.Time       `json:"started_at"`
	FinishedAt    time.Time       `json:"finished_at"`
}

// Normalize applies the persistence boundary for audit data. Metadata events
// retain correlation fields but never business impact or result references.
func Normalize(event Event) Event {
	event.RequestID = sanitizeIdentifier(event.RequestID, MaxRequestIDRunes)
	event.ActorID = sanitizeIdentifier(event.ActorID, MaxActorIDRunes)
	event.ActorType = sanitizeIdentifier(event.ActorType, MaxActorTypeRunes)
	event.Channel = sanitizeIdentifier(event.Channel, MaxChannelRunes)
	event.ActionID = sanitizeIdentifier(event.ActionID, MaxActionIDRunes)
	event.ActionVersion = sanitizeIdentifier(event.ActionVersion, MaxVersionRunes)
	event.ContractHash = sanitizeIdentifier(event.ContractHash, MaxHashRunes)
	event.InputHash = sanitizeIdentifier(event.InputHash, MaxHashRunes)
	event.PlanHash = sanitizeIdentifier(event.PlanHash, MaxHashRunes)
	event.Decision = sanitizeIdentifier(event.Decision, MaxCodeRunes)
	event.AuditLevel = sanitizeIdentifier(event.AuditLevel, MaxCodeRunes)
	event.ErrorCode = sanitizeIdentifier(event.ErrorCode, MaxCodeRunes)
	event.ErrorKind = sanitizeIdentifier(event.ErrorKind, MaxKindRunes)
	event.Scope.Kind = sanitizeIdentifier(event.Scope.Kind, MaxScopeKindRunes)
	event.Scope.ID = sanitizeIdentifier(event.Scope.ID, MaxScopeIDRunes)
	if event.AuditLevel != "detailed" {
		event.AuditLevel = "metadata"
		event.ResultSummary = ""
		event.Impact = nil
		event.ResultRefs = nil
	}
	event.ResultSummary = normalizeFreeText(event.ResultSummary, MaxSummaryRunes)
	event.Reason = normalizeFreeText(event.Reason, MaxReasonRunes)
	if event.Impact != nil {
		impact := *event.Impact
		if impact.Rows < 0 {
			impact.Rows = 0
		}
		impact.Resources = boundedStrings(impact.Resources, MaxResources, MaxResourceRunes)
		event.Impact = &impact
	}
	if len(event.ResultRefs) > MaxReferences {
		event.ResultRefs = event.ResultRefs[:MaxReferences]
	}
	references := make([]Reference, 0, len(event.ResultRefs))
	for _, reference := range event.ResultRefs {
		reference.Kind = sanitizeIdentifier(reference.Kind, MaxKindRunes)
		reference.ID = sanitizeIdentifier(reference.ID, MaxIDRunes)
		if reference.Kind != "" && reference.ID != "" {
			references = append(references, reference)
		}
	}
	event.ResultRefs = references
	return event
}

func normalizeFreeText(value string, limit int) string {
	value = strings.ToValidUTF8(value, "")
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, value)
	return truncate(strings.TrimSpace(value), limit)
}

func boundedStrings(values []string, limit, runeLimit int) []string {
	if len(values) > limit {
		values = values[:limit]
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = sanitizeIdentifier(value, runeLimit); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func sanitizeIdentifier(value string, limit int) string {
	value = strings.ToValidUTF8(value, "")
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	return truncate(strings.TrimSpace(value), limit)
}

func truncate(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}

// Hook persists audit events. Returning nil asserts that the event is durably
// accepted; silently discarding an event violates this contract. Allowed events
// may be called inside the Action transaction and implementations must honor its
// context-bound executor. A returned error is an operational dependency failure
// classified as action.CodeInternal by Runtime. The official F0 implementation
// is adapters/sqlaudit. The same Hook may be called concurrently;
// implementations must be safe for concurrent use, honor context cancellation
// and deadlines, return promptly after cancellation, and treat Event as
// immutable for the duration of the call.
type Hook interface {
	Record(context.Context, Event) error
}
