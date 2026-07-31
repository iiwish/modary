// Package actionpersistence defines the framework-owned persistence boundary
// used by the Action runtime and official storage adapters.
package actionpersistence

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/scope"
	"golang.org/x/mod/semver"
)

const (
	minimumPersistentPlanYear = 1678
	maximumPersistentPlanYear = 2261
)

var canonicalDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ErrPlanNotFound identifies a missing plan in a PlanStore.
var ErrPlanNotFound = errors.New("action plan not found")

// PlanStore stores Preview plans. Get must wrap ErrPlanNotFound when hash is
// absent. Every other store error is an operational dependency failure.
type PlanStore interface {
	Save(context.Context, action.Plan) error
	Get(context.Context, string) (action.Plan, error)
	DeleteExpired(context.Context, time.Time) (int64, error)
}

// IdempotencyStatus identifies the lifecycle state of an idempotency record.
type IdempotencyStatus string

const (
	// IdempotencyRunning marks a reserved execution that has not completed.
	IdempotencyRunning IdempotencyStatus = "running"
	// IdempotencyCompleted marks an execution with a durable stored Result.
	IdempotencyCompleted IdempotencyStatus = "completed"
)

// IdempotencyRecord binds a caller key to one exact Action execution and its result.
type IdempotencyRecord struct {
	Scope               scope.Execution
	ActorID             string
	ActorType           string
	ActionID            string
	ActionVersion       string
	ContractHash        string
	Channel             action.Channel
	Key                 string
	InputHash           string
	PlanHash            string
	Impact              authz.Impact
	DecisionFingerprint string
	Status              IdempotencyStatus
	Result              action.Result
}

// IdempotencyStore looks up, reserves, completes, and aborts execution records
// using the transaction-aware context supplied by Runtime. Store errors are
// operational dependency failures.
type IdempotencyStore interface {
	Lookup(context.Context, IdempotencyRecord) (existing *IdempotencyRecord, err error)
	Reserve(context.Context, IdempotencyRecord) (existing *IdempotencyRecord, err error)
	Complete(context.Context, IdempotencyRecord) error
	Abort(context.Context, IdempotencyRecord) error
}

// ValidatePlanRecord validates the portable persistence contract for a Plan.
// It validates provenance shape, bounded policy fields, JSON, and canonical UTC
// timestamps; Runtime separately verifies that Hash matches the plan material.
func ValidatePlanRecord(plan action.Plan) error {
	if err := action.ValidatePlanHash(plan.Hash); err != nil {
		return err
	}
	if !action.ValidIdentifier(plan.ActionID) {
		return fmt.Errorf("plan action id %q is not a canonical Action identifier", plan.ActionID)
	}
	if err := validatePersistentActionVersion(plan.ActionVersion); err != nil {
		return fmt.Errorf("plan action version %q is invalid", plan.ActionVersion)
	}
	if !canonicalDigestPattern.MatchString(plan.ContractHash) {
		return fmt.Errorf("plan contract hash must be a lowercase SHA-256 digest")
	}
	if err := identity.ValidateActorID(plan.ActorID); err != nil {
		return fmt.Errorf("plan has invalid %w", err)
	}
	if err := identity.ValidateActorType(plan.ActorType); err != nil {
		return fmt.Errorf("plan has invalid %w", err)
	}
	if err := validatePersistenceText("plan channel", string(plan.Channel), true, audit.MaxChannelRunes); err != nil {
		return err
	}
	if err := plan.Scope.Validate(); err != nil {
		return fmt.Errorf("plan scope: %w", err)
	}
	if !canonicalDigestPattern.MatchString(plan.InputHash) {
		return fmt.Errorf("plan input hash must be a lowercase SHA-256 digest")
	}
	if err := action.ValidateJSONDocument(plan.Payload); err != nil {
		return fmt.Errorf("plan payload must contain exactly one JSON value: %w", err)
	}
	if err := validatePersistentImpact(plan.Impact); err != nil {
		return fmt.Errorf("plan impact: %w", err)
	}
	if err := action.ValidateSnapshotHash(plan.SnapshotHash); err != nil {
		return err
	}
	if err := action.ValidateDecisionFingerprint(plan.DecisionFingerprint); err != nil {
		return err
	}
	if err := validatePersistentPlanTime("plan creation time", plan.CreatedAt); err != nil {
		return err
	}
	if err := validatePersistentPlanTime("plan expiry time", plan.ExpiresAt); err != nil {
		return err
	}
	if !plan.ExpiresAt.After(plan.CreatedAt) {
		return fmt.Errorf("plan expiry must be after creation")
	}
	return nil
}

// ValidateIdempotencyLookupRecord validates the identity fields used to look
// up one idempotency binding. PlanHash may be empty before planning.
func ValidateIdempotencyLookupRecord(record IdempotencyRecord) error {
	if err := record.Scope.Validate(); err != nil {
		return fmt.Errorf("idempotency record scope: %w", err)
	}
	if err := identity.ValidateActorID(record.ActorID); err != nil {
		return fmt.Errorf("idempotency record has invalid %w", err)
	}
	if err := identity.ValidateActorType(record.ActorType); err != nil {
		return fmt.Errorf("idempotency record has invalid %w", err)
	}
	if !action.ValidIdentifier(record.ActionID) {
		return fmt.Errorf("idempotency record action id %q is not a canonical Action identifier", record.ActionID)
	}
	if err := validatePersistentActionVersion(record.ActionVersion); err != nil {
		return fmt.Errorf("idempotency record action version %q is invalid", record.ActionVersion)
	}
	if !canonicalDigestPattern.MatchString(record.ContractHash) {
		return fmt.Errorf("idempotency record contract hash must be a lowercase SHA-256 digest")
	}
	if err := validatePersistenceText("idempotency record channel", string(record.Channel), true, audit.MaxChannelRunes); err != nil {
		return err
	}
	if err := action.ValidateIdempotencyKey(record.Key); err != nil {
		return fmt.Errorf("idempotency record has invalid key: %w", err)
	}
	if !canonicalDigestPattern.MatchString(record.InputHash) {
		return fmt.Errorf("idempotency record input hash must be a lowercase SHA-256 digest")
	}
	if record.PlanHash != "" {
		if err := action.ValidatePlanHash(record.PlanHash); err != nil {
			return fmt.Errorf("idempotency record %w", err)
		}
	}
	return nil
}

// ValidateIdempotencyReservationRecord validates a running idempotency record.
func ValidateIdempotencyReservationRecord(record IdempotencyRecord) error {
	if err := ValidateIdempotencyLookupRecord(record); err != nil {
		return err
	}
	if record.PlanHash == "" {
		return fmt.Errorf("idempotency reservation requires a plan hash")
	}
	if err := validatePersistentImpact(record.Impact); err != nil {
		return fmt.Errorf("idempotency record impact: %w", err)
	}
	if err := action.ValidateDecisionFingerprint(record.DecisionFingerprint); err != nil {
		return err
	}
	if record.Status != IdempotencyRunning {
		return fmt.Errorf("idempotency reservation must have status %q", IdempotencyRunning)
	}
	if !resultIsZero(record.Result) {
		return fmt.Errorf("running idempotency record cannot contain a result")
	}
	return nil
}

// ValidateIdempotencyCompletionRecord validates a completed idempotency record
// and its replayable result.
func ValidateIdempotencyCompletionRecord(record IdempotencyRecord) error {
	if err := ValidateIdempotencyLookupRecord(record); err != nil {
		return err
	}
	if record.PlanHash == "" {
		return fmt.Errorf("completed idempotency record requires a plan hash")
	}
	if err := validatePersistentImpact(record.Impact); err != nil {
		return fmt.Errorf("idempotency record impact: %w", err)
	}
	if err := action.ValidateDecisionFingerprint(record.DecisionFingerprint); err != nil {
		return err
	}
	if record.Status != IdempotencyCompleted {
		return fmt.Errorf("completed idempotency record must have status %q", IdempotencyCompleted)
	}
	if err := action.ValidateJSONDocument(record.Result.Data); err != nil {
		return fmt.Errorf("idempotency result data must contain exactly one JSON value: %w", err)
	}
	if err := validatePersistentResult(record.Result); err != nil {
		return fmt.Errorf("idempotency result: %w", err)
	}
	return nil
}

// ValidateStoredIdempotencyRecord validates either durable lifecycle state.
func ValidateStoredIdempotencyRecord(record IdempotencyRecord) error {
	switch record.Status {
	case IdempotencyRunning:
		return ValidateIdempotencyReservationRecord(record)
	case IdempotencyCompleted:
		return ValidateIdempotencyCompletionRecord(record)
	default:
		return fmt.Errorf("stored idempotency status %q is invalid", record.Status)
	}
}

func validatePersistentPlanTime(name string, value time.Time) error {
	if value.IsZero() || value.Year() < minimumPersistentPlanYear || value.Year() > maximumPersistentPlanYear {
		return fmt.Errorf("%s is outside the portable persistence range", name)
	}
	if value.Location() != time.UTC {
		return fmt.Errorf("%s must use UTC", name)
	}
	return nil
}

func resultIsZero(result action.Result) bool {
	return len(result.Data) == 0 && result.Summary == "" && len(result.References) == 0
}

func validatePersistentActionVersion(value string) error {
	if err := validatePersistenceText("action version", value, true, audit.MaxVersionRunes); err != nil {
		return err
	}
	core := value
	if index := strings.IndexAny(core, "-+"); index >= 0 {
		core = core[:index]
	}
	if strings.Count(core, ".") != 2 || !semver.IsValid("v"+value) {
		return fmt.Errorf("action version is not valid Semantic Versioning 2.0.0")
	}
	return nil
}

func validatePersistentImpact(impact authz.Impact) error {
	if impact.Rows < 0 {
		return fmt.Errorf("row count cannot be negative")
	}
	if len(impact.Resources) > audit.MaxResources {
		return fmt.Errorf("resource count exceeds %d", audit.MaxResources)
	}
	seen := make(map[string]struct{}, len(impact.Resources))
	for _, resource := range impact.Resources {
		if err := validatePersistenceText("resource identifier", resource, true, audit.MaxResourceRunes); err != nil {
			return err
		}
		if _, exists := seen[resource]; exists {
			return fmt.Errorf("resource identifier %q is duplicated", resource)
		}
		seen[resource] = struct{}{}
	}
	return nil
}

func validatePersistentResult(result action.Result) error {
	if err := validatePersistenceText("result summary", result.Summary, false, audit.MaxSummaryRunes); err != nil {
		return err
	}
	if len(result.References) > audit.MaxReferences {
		return fmt.Errorf("result reference count exceeds %d", audit.MaxReferences)
	}
	seen := make(map[audit.Reference]struct{}, len(result.References))
	for _, reference := range result.References {
		if err := validatePersistenceToken("result reference kind", reference.Kind, true, audit.MaxKindRunes); err != nil {
			return err
		}
		if err := validatePersistenceToken("result reference id", reference.ID, true, audit.MaxIDRunes); err != nil {
			return err
		}
		if _, exists := seen[reference]; exists {
			return fmt.Errorf("result reference is duplicated")
		}
		seen[reference] = struct{}{}
	}
	return nil
}

func validatePersistenceToken(field, value string, required bool, maxRunes int) error {
	if err := validatePersistenceText(field, value, required, maxRunes); err != nil {
		return err
	}
	if strings.ContainsFunc(value, unicode.IsSpace) {
		return fmt.Errorf("%s cannot contain whitespace", field)
	}
	return nil
}

func validatePersistenceText(field, value string, required bool, maxRunes int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", field)
	}
	if required && value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s cannot contain surrounding whitespace", field)
	}
	if utf8.RuneCountInString(value) > maxRunes {
		return fmt.Errorf("%s cannot exceed %d characters", field, maxRunes)
	}
	if strings.ContainsFunc(value, unicode.IsControl) {
		return fmt.Errorf("%s cannot contain control characters", field)
	}
	return nil
}
