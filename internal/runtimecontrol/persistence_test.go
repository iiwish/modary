package runtimecontrol

import (
	"context"
	"testing"

	"github.com/iiwish/modary/internal/actionpersistence"
	"github.com/iiwish/modary/internal/testsupport"
)

type pointerTransactions struct{}

func (*pointerTransactions) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	return operation(ctx)
}

func TestNewRejectsMissingAndTypedNilBundleMembers(t *testing.T) {
	validPlans := testsupport.NewMemoryPlanStore()
	validIdempotency := testsupport.NewMemoryIdempotencyStore()
	validTransactions := &pointerTransactions{}

	var nilPlans *testsupport.MemoryPlanStore
	var nilIdempotency *testsupport.MemoryIdempotencyStore
	var nilTransactions *pointerTransactions
	for _, test := range []struct {
		name         string
		plans        actionpersistence.PlanStore
		idempotency  actionpersistence.IdempotencyStore
		transactions TransactionManager
	}{
		{name: "missing plans", idempotency: validIdempotency, transactions: validTransactions},
		{name: "typed nil plans", plans: nilPlans, idempotency: validIdempotency, transactions: validTransactions},
		{name: "missing idempotency", plans: validPlans, transactions: validTransactions},
		{name: "typed nil idempotency", plans: validPlans, idempotency: nilIdempotency, transactions: validTransactions},
		{name: "missing transactions", plans: validPlans, idempotency: validIdempotency},
		{name: "typed nil transactions", plans: validPlans, idempotency: validIdempotency, transactions: nilTransactions},
	} {
		t.Run(test.name, func(t *testing.T) {
			persistence, err := New(test.plans, test.idempotency, test.transactions)
			if err == nil || persistence != nil {
				t.Fatalf("New() = %#v, %v; want nil, error", persistence, err)
			}
		})
	}
}

func TestNewSealsOneCompleteBundleWithoutReplacingMembers(t *testing.T) {
	plans := testsupport.NewMemoryPlanStore()
	idempotency := testsupport.NewMemoryIdempotencyStore()
	transactions := &pointerTransactions{}
	bundle, err := New(plans, idempotency, transactions)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if bundle.Plans() != plans || bundle.Idempotency() != idempotency || bundle.Transactions() != transactions {
		t.Fatalf("bundle members were replaced: %#v", bundle)
	}

	var nilPersistence *persistence
	if nilPersistence.Plans() != nil || nilPersistence.Idempotency() != nil || nilPersistence.Transactions() != nil {
		t.Fatal("nil persistence receiver exposed a member")
	}
}
