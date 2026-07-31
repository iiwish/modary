package actionruntime

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	. "github.com/iiwish/modary/action"
	. "github.com/iiwish/modary/internal/actionpersistence"
)

type testPlanStore struct {
	mu    sync.RWMutex
	plans map[string]Plan
}

type negativeDeletePlanStore struct{ *testPlanStore }

func (store *negativeDeletePlanStore) DeleteExpired(context.Context, time.Time) (int64, error) {
	return -1, nil
}

func newMemoryPlanStore() *testPlanStore {
	return &testPlanStore{plans: make(map[string]Plan)}
}

func (store *testPlanStore) Save(_ context.Context, plan Plan) error {
	if err := ValidatePlanRecord(plan); err != nil {
		return err
	}
	store.mu.Lock()
	store.plans[plan.Hash] = clonePlan(plan)
	store.mu.Unlock()
	return nil
}

func (store *testPlanStore) Get(_ context.Context, hash string) (Plan, error) {
	store.mu.RLock()
	plan, ok := store.plans[hash]
	store.mu.RUnlock()
	if !ok {
		return Plan{}, fmt.Errorf("plan %s: %w", hash, ErrPlanNotFound)
	}
	return clonePlan(plan), nil
}

func (store *testPlanStore) DeleteExpired(_ context.Context, before time.Time) (int64, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	var deleted int64
	for hash, plan := range store.plans {
		if !plan.ExpiresAt.After(before) {
			delete(store.plans, hash)
			deleted++
		}
	}
	return deleted, nil
}

type testIdempotencyKey struct {
	scopeKind string
	scopeID   string
	actorID   string
	actorType string
	actionID  string
	key       string
}

type testIdempotencyStore struct {
	mu      sync.Mutex
	records map[testIdempotencyKey]IdempotencyRecord
}

type maliciousIdempotencyStore struct{ record IdempotencyRecord }

func (store *maliciousIdempotencyStore) Lookup(context.Context, IdempotencyRecord) (*IdempotencyRecord, error) {
	record := cloneIdempotencyRecord(store.record)
	return &record, nil
}

func (store *maliciousIdempotencyStore) Reserve(context.Context, IdempotencyRecord) (*IdempotencyRecord, error) {
	panic("malicious store Reserve must not run during replay")
}

func (store *maliciousIdempotencyStore) Complete(context.Context, IdempotencyRecord) error {
	panic("malicious store Complete must not run during replay")
}

func (store *maliciousIdempotencyStore) Abort(context.Context, IdempotencyRecord) error {
	panic("malicious store Abort must not run during replay")
}

func newMemoryIdempotencyStore() *testIdempotencyStore {
	return &testIdempotencyStore{records: make(map[testIdempotencyKey]IdempotencyRecord)}
}

func (store *testIdempotencyStore) Lookup(_ context.Context, record IdempotencyRecord) (*IdempotencyRecord, error) {
	if err := ValidateIdempotencyLookupRecord(record); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	existing, ok := store.records[testRecordKey(record)]
	if !ok {
		return nil, nil
	}
	copy := cloneIdempotencyRecord(existing)
	return &copy, nil
}

func (store *testIdempotencyStore) Reserve(_ context.Context, record IdempotencyRecord) (*IdempotencyRecord, error) {
	if err := ValidateIdempotencyReservationRecord(record); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	key := testRecordKey(record)
	if existing, ok := store.records[key]; ok {
		copy := cloneIdempotencyRecord(existing)
		return &copy, nil
	}
	store.records[key] = cloneIdempotencyRecord(record)
	return nil, nil
}

func (store *testIdempotencyStore) Complete(_ context.Context, record IdempotencyRecord) error {
	if err := ValidateIdempotencyCompletionRecord(record); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	key := testRecordKey(record)
	existing, ok := store.records[key]
	if !ok || existing.Status != IdempotencyRunning || !sameTestExecution(existing, record) {
		return fmt.Errorf("idempotency record has no matching running reservation")
	}
	store.records[key] = cloneIdempotencyRecord(record)
	return nil
}

func (store *testIdempotencyStore) Abort(_ context.Context, record IdempotencyRecord) error {
	if err := ValidateIdempotencyReservationRecord(record); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	key := testRecordKey(record)
	if existing, ok := store.records[key]; ok && existing.Status == IdempotencyRunning && sameTestExecution(existing, record) {
		delete(store.records, key)
	}
	return nil
}

func testRecordKey(record IdempotencyRecord) testIdempotencyKey {
	return testIdempotencyKey{
		scopeKind: record.Scope.Kind, scopeID: record.Scope.ID,
		actorID: record.ActorID, actorType: record.ActorType,
		actionID: record.ActionID, key: record.Key,
	}
}

func sameTestExecution(first, second IdempotencyRecord) bool {
	return first.Scope == second.Scope && first.ActorID == second.ActorID &&
		first.ActorType == second.ActorType && first.ActionID == second.ActionID &&
		first.ActionVersion == second.ActionVersion && first.ContractHash == second.ContractHash &&
		first.Channel == second.Channel && first.Key == second.Key && first.InputHash == second.InputHash &&
		first.PlanHash == second.PlanHash && first.Impact.Rows == second.Impact.Rows &&
		slices.Equal(first.Impact.Resources, second.Impact.Resources) &&
		first.DecisionFingerprint == second.DecisionFingerprint
}
