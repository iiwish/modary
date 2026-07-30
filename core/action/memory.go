package action

import (
	"context"
	"fmt"
	"sync"
)

type MemoryPlanStore struct {
	mu    sync.RWMutex
	plans map[string]Plan
}

func NewMemoryPlanStore() *MemoryPlanStore {
	return &MemoryPlanStore{plans: make(map[string]Plan)}
}

func (s *MemoryPlanStore) Save(_ context.Context, plan Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.plans[plan.Hash] = plan
	return nil
}

func (s *MemoryPlanStore) Get(_ context.Context, hash string) (Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	plan, ok := s.plans[hash]
	if !ok {
		return Plan{}, fmt.Errorf("plan %s not found", hash)
	}
	return plan, nil
}

type MemoryIdempotencyStore struct {
	mu      sync.Mutex
	records map[string]IdempotencyRecord
}

func NewMemoryIdempotencyStore() *MemoryIdempotencyStore {
	return &MemoryIdempotencyStore{records: make(map[string]IdempotencyRecord)}
}

func (s *MemoryIdempotencyStore) Reserve(_ context.Context, record IdempotencyRecord) (*IdempotencyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := idempotencyMapKey(record)
	if existing, ok := s.records[key]; ok {
		copy := existing
		return &copy, nil
	}
	record.Status = "running"
	s.records[key] = record
	return nil, nil
}

func (s *MemoryIdempotencyStore) Complete(_ context.Context, record IdempotencyRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record.Status = "completed"
	s.records[idempotencyMapKey(record)] = record
	return nil
}

func idempotencyMapKey(record IdempotencyRecord) string {
	return record.WorkspaceID + "\x00" + record.ActorID + "\x00" + record.ActionID + "\x00" + record.Key
}

type NopTransactionManager struct{}

func (NopTransactionManager) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
