package governedpostgres

import (
	"database/sql"
	"fmt"
	"unicode/utf8"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/internal/actionpersistence"
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

type projectedPlan struct {
	planHash, actionID, actionVersion, contractHash sql.NullString
	actorID, actorType, channel                     sql.NullString
	scopeKind, scopeID, inputHash                   sql.NullString
	impactRows                                      sql.NullInt64
	snapshotHash, fingerprint                       sql.NullString
	createdAt, expiresAt                            sql.NullString
	expiresAtUnixNano                               sql.NullInt64
}

type projectedIdempotencyRecord struct {
	scopeKind, scopeID, actorID, actorType sql.NullString
	actionID, actionVersion, contractHash  sql.NullString
	channel, key, inputHash, planHash      sql.NullString
	impactRows                             sql.NullInt64
	fingerprint, status                    sql.NullString
	resultLength                           sql.NullInt64
	resultSummary, createdAt, updatedAt    sql.NullString
}

func populateProjectedPlan(plan *action.Plan, value projectedPlan) error {
	var err error
	if plan.Hash, err = requireProjectedText(value.planHash, "Action plan hash"); err != nil {
		return err
	}
	if plan.ActionID, err = requireProjectedText(value.actionID, "Action plan action id"); err != nil {
		return err
	}
	if plan.ActionVersion, err = requireProjectedText(value.actionVersion, "Action plan action version"); err != nil {
		return err
	}
	if plan.ContractHash, err = requireProjectedText(value.contractHash, "Action plan contract hash"); err != nil {
		return err
	}
	if plan.ActorID, err = requireProjectedText(value.actorID, "Action plan actor id"); err != nil {
		return err
	}
	if plan.ActorType, err = requireProjectedText(value.actorType, "Action plan actor type"); err != nil {
		return err
	}
	channel, err := requireProjectedText(value.channel, "Action plan channel")
	if err != nil {
		return err
	}
	plan.Channel = action.Channel(channel)
	if plan.Scope.Kind, err = requireProjectedText(value.scopeKind, "Action plan scope kind"); err != nil {
		return err
	}
	if plan.Scope.ID, err = requireProjectedText(value.scopeID, "Action plan scope id"); err != nil {
		return err
	}
	if plan.InputHash, err = requireProjectedText(value.inputHash, "Action plan input hash"); err != nil {
		return err
	}
	impactRows, err := requireProjectedInteger(value.impactRows, "Action plan impact rows")
	if err != nil {
		return err
	}
	plan.Impact.Rows = int(impactRows)
	if int64(plan.Impact.Rows) != impactRows {
		return fmt.Errorf("stored Action plan impact rows exceed the platform integer range")
	}
	if plan.SnapshotHash, err = requireProjectedText(value.snapshotHash, "Action plan snapshot hash"); err != nil {
		return err
	}
	if plan.DecisionFingerprint, err = requireProjectedText(value.fingerprint, "Action plan decision fingerprint"); err != nil {
		return err
	}
	if _, err = requireProjectedText(value.createdAt, "Action plan creation time"); err != nil {
		return err
	}
	if _, err = requireProjectedText(value.expiresAt, "Action plan expiry time"); err != nil {
		return err
	}
	_, err = requireProjectedInteger(value.expiresAtUnixNano, "Action plan expiry epoch")
	return err
}

func populateProjectedIdempotencyRecord(record *actionpersistence.IdempotencyRecord, value projectedIdempotencyRecord) error {
	var err error
	if record.Scope.Kind, err = requireProjectedText(value.scopeKind, "idempotency scope kind"); err != nil {
		return err
	}
	if record.Scope.ID, err = requireProjectedText(value.scopeID, "idempotency scope id"); err != nil {
		return err
	}
	if record.ActorID, err = requireProjectedText(value.actorID, "idempotency actor id"); err != nil {
		return err
	}
	if record.ActorType, err = requireProjectedText(value.actorType, "idempotency actor type"); err != nil {
		return err
	}
	if record.ActionID, err = requireProjectedText(value.actionID, "idempotency action id"); err != nil {
		return err
	}
	if record.ActionVersion, err = requireProjectedText(value.actionVersion, "idempotency action version"); err != nil {
		return err
	}
	if record.ContractHash, err = requireProjectedText(value.contractHash, "idempotency contract hash"); err != nil {
		return err
	}
	channel, err := requireProjectedText(value.channel, "idempotency channel")
	if err != nil {
		return err
	}
	record.Channel = action.Channel(channel)
	if record.Key, err = requireProjectedText(value.key, "idempotency key"); err != nil {
		return err
	}
	if record.InputHash, err = requireProjectedText(value.inputHash, "idempotency input hash"); err != nil {
		return err
	}
	if record.PlanHash, err = requireProjectedText(value.planHash, "idempotency plan hash"); err != nil {
		return err
	}
	impactRows, err := requireProjectedInteger(value.impactRows, "idempotency impact rows")
	if err != nil {
		return err
	}
	record.Impact.Rows = int(impactRows)
	if int64(record.Impact.Rows) != impactRows {
		return fmt.Errorf("stored idempotency impact rows exceed the platform integer range")
	}
	if record.DecisionFingerprint, err = requireProjectedText(value.fingerprint, "idempotency decision fingerprint"); err != nil {
		return err
	}
	status, err := requireProjectedText(value.status, "idempotency status")
	if err != nil {
		return err
	}
	record.Status = actionpersistence.IdempotencyStatus(status)
	if record.Result.Summary, err = requireProjectedText(value.resultSummary, "idempotency result summary"); err != nil {
		return err
	}
	if _, err = requireProjectedText(value.createdAt, "idempotency creation time"); err != nil {
		return err
	}
	_, err = requireProjectedText(value.updatedAt, "idempotency update time")
	return err
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
