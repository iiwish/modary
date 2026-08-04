package governedpostgres

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/internal/actionpersistence"
)

func normalizePlan(plan action.Plan) (action.Plan, error) {
	if int64(len(plan.Payload)) > action.MaxJSONDocumentBytes {
		return action.Plan{}, fmt.Errorf("plan payload exceeds %d bytes", action.MaxJSONDocumentBytes)
	}
	plan.Payload = append(json.RawMessage(nil), plan.Payload...)
	plan.Impact.Resources = append([]string(nil), plan.Impact.Resources...)
	if err := actionpersistence.ValidatePlanRecord(plan); err != nil {
		return action.Plan{}, err
	}
	return plan, nil
}

func validateLookupRecord(record actionpersistence.IdempotencyRecord) error {
	return actionpersistence.ValidateIdempotencyLookupRecord(record)
}

func normalizeReservation(record actionpersistence.IdempotencyRecord) (actionpersistence.IdempotencyRecord, error) {
	record = cloneIdempotencyRecord(record)
	if err := actionpersistence.ValidateIdempotencyReservationRecord(record); err != nil {
		return actionpersistence.IdempotencyRecord{}, err
	}
	return record, nil
}

func normalizeCompletion(record actionpersistence.IdempotencyRecord) (actionpersistence.IdempotencyRecord, error) {
	if int64(len(record.Result.Data)) > action.MaxJSONDocumentBytes {
		return actionpersistence.IdempotencyRecord{}, fmt.Errorf("idempotency result exceeds %d bytes", action.MaxJSONDocumentBytes)
	}
	record = cloneIdempotencyRecord(record)
	if err := actionpersistence.ValidateIdempotencyCompletionRecord(record); err != nil {
		return actionpersistence.IdempotencyRecord{}, err
	}
	return record, nil
}

func normalizeStoredRecord(record actionpersistence.IdempotencyRecord) (actionpersistence.IdempotencyRecord, error) {
	record = cloneIdempotencyRecord(record)
	if err := actionpersistence.ValidateStoredIdempotencyRecord(record); err != nil {
		return actionpersistence.IdempotencyRecord{}, err
	}
	return record, nil
}

func validateStoredTime(name string, value time.Time) error {
	if value.IsZero() || value.Year() < 1678 || value.Year() > 2261 || value.Location() != time.UTC {
		return fmt.Errorf("%s is outside the durable PostgreSQL timestamp range", name)
	}
	return nil
}

func parseStoredTime(name, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Location() != time.UTC || parsed.Format(time.RFC3339Nano) != value {
		return time.Time{}, fmt.Errorf("stored %s is not canonical UTC RFC3339Nano", name)
	}
	if err := validateStoredTime(name, parsed); err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}

func cloneIdempotencyRecord(record actionpersistence.IdempotencyRecord) actionpersistence.IdempotencyRecord {
	record.Impact.Resources = append([]string(nil), record.Impact.Resources...)
	record.Result.Data = append(json.RawMessage(nil), record.Result.Data...)
	record.Result.References = append([]audit.Reference(nil), record.Result.References...)
	return record
}
