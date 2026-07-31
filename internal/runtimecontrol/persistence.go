// Package runtimecontrol owns the privileged Action persistence assembly
// contract. It is internal because F0 supports only framework-owned durable
// adapters whose transaction outcomes can be proven to the Runtime.
package runtimecontrol

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/iiwish/modary/internal/actionpersistence"
)

// ServiceName is the reserved Host service used for the atomic persistence
// bundle. Consumers cannot name its sealed type or recreate its service key.
const ServiceName = "modary.action-persistence"

// ErrTransactionManagerContract identifies an official transaction owner that
// did not execute and propagate its callback according to the Runtime contract.
var ErrTransactionManagerContract = errors.New("action transaction manager contract violation")

// TransactionManager is the private commit-or-rollback capability consumed by
// the governed Runtime. Official adapters must return framework-correlated
// outcome proof when an operation does not complete normally.
type TransactionManager interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}

// Persistence is the sealed, atomic persistence bundle installed by one
// official adapter. Keeping the three services together prevents mixed owners
// and prevents consumers from installing transaction control they cannot
// implement with the private outcome protocol.
type Persistence interface {
	Plans() actionpersistence.PlanStore
	Idempotency() actionpersistence.IdempotencyStore
	Transactions() TransactionManager
	runtimePersistence()
}

type persistence struct {
	plans        actionpersistence.PlanStore
	idempotency  actionpersistence.IdempotencyStore
	transactions TransactionManager
}

// New validates and seals one complete Action persistence bundle.
func New(plans actionpersistence.PlanStore, idempotency actionpersistence.IdempotencyStore, transactions TransactionManager) (Persistence, error) {
	if isNil(plans) {
		return nil, fmt.Errorf("plan store is required")
	}
	if isNil(idempotency) {
		return nil, fmt.Errorf("idempotency store is required")
	}
	if isNil(transactions) {
		return nil, fmt.Errorf("transaction manager is required")
	}
	return &persistence{plans: plans, idempotency: idempotency, transactions: transactions}, nil
}

func (*persistence) runtimePersistence() {}

func (value *persistence) Plans() actionpersistence.PlanStore {
	if value == nil {
		return nil
	}
	return value.plans
}

func (value *persistence) Idempotency() actionpersistence.IdempotencyStore {
	if value == nil {
		return nil
	}
	return value.idempotency
}

func (value *persistence) Transactions() TransactionManager {
	if value == nil {
		return nil
	}
	return value.transactions
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
