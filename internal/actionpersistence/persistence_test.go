package actionpersistence

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/scope"
)

func TestPlanRecordUsesCanonicalSnapshotDigestAndFingerprintBound(t *testing.T) {
	plan := validPersistentPlanForTest()
	plan.DecisionFingerprint = strings.Repeat("界", authz.MaxFingerprintRunes)
	if err := ValidatePlanRecord(plan); err != nil {
		t.Fatalf("maximum fingerprint rejected: %v", err)
	}
	plan.DecisionFingerprint += "界"
	if err := ValidatePlanRecord(plan); err == nil || !strings.Contains(err.Error(), "cannot exceed") {
		t.Fatalf("oversized fingerprint error = %v", err)
	}
}

func TestPlanRecordRequiresCanonicalPortableTimestamps(t *testing.T) {
	plan := validPersistentPlanForTest()
	plan.CreatedAt = plan.CreatedAt.In(time.FixedZone("UTC-alias", 0))
	if err := ValidatePlanRecord(plan); err == nil || !strings.Contains(err.Error(), "must use UTC") {
		t.Fatalf("non-canonical location error = %v", err)
	}
	plan = validPersistentPlanForTest()
	plan.CreatedAt = time.Date(2500, 1, 1, 0, 0, 0, 0, time.UTC)
	plan.ExpiresAt = plan.CreatedAt.Add(time.Minute)
	if err := ValidatePlanRecord(plan); err == nil || !strings.Contains(err.Error(), "portable persistence range") {
		t.Fatalf("out-of-range time error = %v", err)
	}
}

func TestPersistenceRecordsEnforceActionJSONResourceLimits(t *testing.T) {
	for _, boundary := range persistenceJSONBoundaries() {
		t.Run("plan payload/"+boundary.name, func(t *testing.T) {
			plan := validPersistentPlanForTest()
			plan.Payload = boundary.value
			err := ValidatePlanRecord(plan)
			if boundary.within && err != nil {
				t.Fatalf("exact persisted plan boundary rejected: %v", err)
			}
			if !boundary.within && err == nil {
				t.Fatal("persisted plan above boundary was accepted")
			}
		})
		t.Run("idempotency result/"+boundary.name, func(t *testing.T) {
			completion := validPersistentCompletionForTest()
			completion.Result.Data = boundary.value
			err := ValidateIdempotencyCompletionRecord(completion)
			if boundary.within && err != nil {
				t.Fatalf("exact persisted result boundary rejected: %v", err)
			}
			if !boundary.within && err == nil {
				t.Fatal("persisted result above boundary was accepted")
			}
		})
	}
}

type persistenceJSONBoundary struct {
	name   string
	value  json.RawMessage
	within bool
}

func persistenceJSONBoundaries() []persistenceJSONBoundary {
	return []persistenceJSONBoundary{
		{name: "bytes exact", value: json.RawMessage(`"` + strings.Repeat("x", int(action.MaxJSONDocumentBytes)-2) + `"`), within: true},
		{name: "bytes above", value: json.RawMessage(`"` + strings.Repeat("x", int(action.MaxJSONDocumentBytes)-1) + `"`)},
		{name: "depth exact", value: persistentNestedJSON(action.MaxJSONNestingDepth), within: true},
		{name: "depth above", value: persistentNestedJSON(action.MaxJSONNestingDepth + 1)},
		{name: "nodes exact", value: persistentArrayJSON(action.MaxJSONValueNodes - 1), within: true},
		{name: "nodes above", value: persistentArrayJSON(action.MaxJSONValueNodes)},
		{name: "number exact", value: persistentNumberJSON(action.MaxJSONNumberBytes), within: true},
		{name: "number above", value: persistentNumberJSON(action.MaxJSONNumberBytes + 1)},
	}
}

func persistentNestedJSON(depth int) json.RawMessage {
	return json.RawMessage(strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth))
}

func persistentArrayJSON(values int) json.RawMessage {
	return json.RawMessage("[" + strings.TrimSuffix(strings.Repeat("0,", values), ",") + "]")
}

func persistentNumberJSON(bytes int) json.RawMessage {
	return json.RawMessage("1" + strings.Repeat("0", bytes-1))
}

func TestIdempotencyRecordUsesPublicPortableKeyContract(t *testing.T) {
	record := validPersistentIdempotencyForTest()
	for _, key := range []string{" retry", "retry key", "界", strings.Repeat("x", action.MaxIdempotencyKeyBytes+1)} {
		record.Key = key
		if err := ValidateIdempotencyLookupRecord(record); err == nil {
			t.Fatalf("non-portable key %q was accepted", key)
		}
	}
}

func validPersistentPlanForTest() action.Plan {
	createdAt := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	return action.Plan{
		Hash:                persistentDigestForTest('a'),
		ActionID:            "resource.write",
		ActionVersion:       "1.2.3",
		ContractHash:        persistentDigestForTest('b'),
		ActorID:             "person-123",
		ActorType:           "human",
		Channel:             action.ChannelHTTP,
		Scope:               scope.Must("account", "alpha"),
		InputHash:           persistentDigestForTest('c'),
		Payload:             json.RawMessage(`{"value":1}`),
		Impact:              authz.Impact{Rows: 1, Resources: []string{"resource/one"}},
		SnapshotHash:        persistentDigestForTest('d'),
		DecisionFingerprint: "grant:v1",
		CreatedAt:           createdAt,
		ExpiresAt:           createdAt.Add(time.Minute),
	}
}

func validPersistentIdempotencyForTest() IdempotencyRecord {
	plan := validPersistentPlanForTest()
	return IdempotencyRecord{
		Scope:               plan.Scope,
		ActorID:             plan.ActorID,
		ActorType:           plan.ActorType,
		ActionID:            plan.ActionID,
		ActionVersion:       plan.ActionVersion,
		ContractHash:        plan.ContractHash,
		Channel:             plan.Channel,
		Key:                 "once",
		InputHash:           plan.InputHash,
		PlanHash:            plan.Hash,
		Impact:              plan.Impact,
		DecisionFingerprint: plan.DecisionFingerprint,
		Status:              IdempotencyRunning,
		Result:              action.Result{},
	}
}

func validPersistentCompletionForTest() IdempotencyRecord {
	record := validPersistentIdempotencyForTest()
	record.Status = IdempotencyCompleted
	record.Result = action.Result{
		Data:       json.RawMessage(`{"ok":true}`),
		Summary:    "complete",
		References: []audit.Reference{{Kind: "resource", ID: "one"}},
	}
	return record
}

func persistentDigestForTest(character byte) string {
	return "sha256:" + strings.Repeat(string(character), 64)
}
