package testsupport

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/internal/actionpersistence"
)

// MemoryPlanStore is a non-durable test PlanStore.
type MemoryPlanStore struct {
	mu    sync.RWMutex
	plans map[string]action.Plan
}

func NewMemoryPlanStore() *MemoryPlanStore {
	return &MemoryPlanStore{plans: make(map[string]action.Plan)}
}

func (store *MemoryPlanStore) Save(_ context.Context, plan action.Plan) error {
	if err := actionpersistence.ValidatePlanRecord(plan); err != nil {
		return err
	}
	store.mu.Lock()
	store.plans[plan.Hash] = clonePlan(plan)
	store.mu.Unlock()
	return nil
}

func (store *MemoryPlanStore) Get(_ context.Context, hash string) (action.Plan, error) {
	store.mu.RLock()
	plan, ok := store.plans[hash]
	store.mu.RUnlock()
	if !ok {
		return action.Plan{}, fmt.Errorf("plan %s: %w", hash, actionpersistence.ErrPlanNotFound)
	}
	return clonePlan(plan), nil
}

func (store *MemoryPlanStore) DeleteExpired(_ context.Context, before time.Time) (int64, error) {
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

// MemoryIdempotencyStore is a non-durable test IdempotencyStore.
type MemoryIdempotencyStore struct {
	mu      sync.Mutex
	records map[idempotencyKey]actionpersistence.IdempotencyRecord
}

type idempotencyKey struct {
	scopeKind string
	scopeID   string
	actorID   string
	actorType string
	actionID  string
	key       string
}

func NewMemoryIdempotencyStore() *MemoryIdempotencyStore {
	return &MemoryIdempotencyStore{records: make(map[idempotencyKey]actionpersistence.IdempotencyRecord)}
}

func (store *MemoryIdempotencyStore) Lookup(_ context.Context, record actionpersistence.IdempotencyRecord) (*actionpersistence.IdempotencyRecord, error) {
	if err := actionpersistence.ValidateIdempotencyLookupRecord(record); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	existing, ok := store.records[memoryRecordKey(record)]
	if !ok {
		return nil, nil
	}
	clone := cloneIdempotencyRecord(existing)
	return &clone, nil
}

func (store *MemoryIdempotencyStore) Reserve(_ context.Context, record actionpersistence.IdempotencyRecord) (*actionpersistence.IdempotencyRecord, error) {
	if err := actionpersistence.ValidateIdempotencyReservationRecord(record); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	key := memoryRecordKey(record)
	if existing, ok := store.records[key]; ok {
		clone := cloneIdempotencyRecord(existing)
		return &clone, nil
	}
	store.records[key] = cloneIdempotencyRecord(record)
	return nil, nil
}

func (store *MemoryIdempotencyStore) Complete(_ context.Context, record actionpersistence.IdempotencyRecord) error {
	if err := actionpersistence.ValidateIdempotencyCompletionRecord(record); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	key := memoryRecordKey(record)
	existing, ok := store.records[key]
	if !ok || existing.Status != actionpersistence.IdempotencyRunning || !sameExecution(existing, record) {
		return fmt.Errorf("idempotency record has no matching running reservation")
	}
	store.records[key] = cloneIdempotencyRecord(record)
	return nil
}

func (store *MemoryIdempotencyStore) Abort(_ context.Context, record actionpersistence.IdempotencyRecord) error {
	if err := actionpersistence.ValidateIdempotencyReservationRecord(record); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	key := memoryRecordKey(record)
	if existing, ok := store.records[key]; ok && existing.Status == actionpersistence.IdempotencyRunning && sameExecution(existing, record) {
		delete(store.records, key)
	}
	return nil
}

func memoryRecordKey(record actionpersistence.IdempotencyRecord) idempotencyKey {
	return idempotencyKey{
		scopeKind: record.Scope.Kind, scopeID: record.Scope.ID,
		actorID: record.ActorID, actorType: record.ActorType,
		actionID: record.ActionID, key: record.Key,
	}
}

func sameExecution(first, second actionpersistence.IdempotencyRecord) bool {
	return first.Scope == second.Scope && first.ActorID == second.ActorID &&
		first.ActorType == second.ActorType && first.ActionID == second.ActionID &&
		first.ActionVersion == second.ActionVersion && first.ContractHash == second.ContractHash &&
		first.Channel == second.Channel && first.Key == second.Key && first.InputHash == second.InputHash &&
		first.PlanHash == second.PlanHash && first.Impact.Rows == second.Impact.Rows &&
		slices.Equal(first.Impact.Resources, second.Impact.Resources) &&
		first.DecisionFingerprint == second.DecisionFingerprint
}

func clonePlan(plan action.Plan) action.Plan {
	plan.Payload = append([]byte(nil), plan.Payload...)
	plan.Impact.Resources = append([]string(nil), plan.Impact.Resources...)
	return plan
}

func cloneIdempotencyRecord(record actionpersistence.IdempotencyRecord) actionpersistence.IdempotencyRecord {
	record.Impact.Resources = append([]string(nil), record.Impact.Resources...)
	record.Result.Data = append([]byte(nil), record.Result.Data...)
	record.Result.References = append([]audit.Reference(nil), record.Result.References...)
	return record
}
