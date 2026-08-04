package governedpostgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/internal/actionpersistence"
	"github.com/iiwish/modary/scope"
)

func TestPlanStoreRoundTripImmutabilityAndExpiry(t *testing.T) {
	services := startTestServices(t)
	ctx := context.Background()
	plan := validPlan()
	want := clonePlanForTest(plan)
	if err := services.plans.Save(ctx, plan); err != nil {
		t.Fatal(err)
	}
	plan.Payload[0] = '['
	plan.Impact.Resources[0] = "mutated"
	got, err := services.plans.Get(ctx, want.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stored plan = %#v, want %#v", got, want)
	}
	if err := services.plans.Save(ctx, want); err != nil {
		t.Fatalf("idempotent Save: %v", err)
	}
	conflict := clonePlanForTest(want)
	conflict.ActorID = "different-actor"
	if err := services.plans.Save(ctx, conflict); err == nil {
		t.Fatal("same plan hash replaced different provenance")
	}
	if _, err := services.plans.Get(ctx, digest('f')); !errors.Is(err, actionpersistence.ErrPlanNotFound) {
		t.Fatalf("missing plan error = %v", err)
	}

	expired := clonePlanForTest(want)
	expired.Hash = digest('0')
	expired.CreatedAt = want.CreatedAt.Add(-time.Hour)
	expired.ExpiresAt = want.CreatedAt.Add(-time.Nanosecond)
	if err := services.plans.Save(ctx, expired); err != nil {
		t.Fatal(err)
	}
	deleted, err := services.plans.DeleteExpired(ctx, want.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted plans = %d", deleted)
	}
	if _, err := services.plans.Get(ctx, expired.Hash); !errors.Is(err, actionpersistence.ErrPlanNotFound) {
		t.Fatalf("expired plan remains: %v", err)
	}
	if _, err := services.plans.Get(ctx, want.Hash); err != nil {
		t.Fatalf("active plan was deleted: %v", err)
	}
}

func TestPostgresStoresEnforcePublishedActionJSONBoundaryMatrix(t *testing.T) {
	services := startTestServices(t)
	ctx := context.Background()
	for index, boundary := range postgresJSONBoundaries() {
		t.Run(boundary.name, func(t *testing.T) {
			plan := validPlan()
			plan.Hash = digest(byte('1' + index))
			plan.Payload = boundary.value
			planErr := services.plans.Save(ctx, plan)

			record := validReservation()
			record.Key = fmt.Sprintf("json-boundary-%d", index)
			record.PlanHash = plan.Hash
			if existing, err := services.idempotency.Reserve(ctx, record); err != nil || existing != nil {
				t.Fatalf("reserve boundary record = %#v, %v", existing, err)
			}
			completion := cloneRecordForTest(record)
			completion.Status = actionpersistence.IdempotencyCompleted
			completion.Result = action.Result{Data: boundary.value}
			completionErr := services.idempotency.Complete(ctx, completion)

			if !boundary.within {
				if planErr == nil || completionErr == nil {
					t.Fatalf("above boundary persisted: plan=%v completion=%v", planErr, completionErr)
				}
				if _, err := services.plans.Get(ctx, plan.Hash); !errors.Is(err, actionpersistence.ErrPlanNotFound) {
					t.Fatalf("rejected plan left persistent state: %v", err)
				}
				loadedRecord, err := services.idempotency.Lookup(ctx, record)
				if err != nil || loadedRecord == nil || !reflect.DeepEqual(*loadedRecord, record) {
					t.Fatalf("rejected completion changed running reservation: %#v, %v", loadedRecord, err)
				}
				return
			}
			if planErr != nil || completionErr != nil {
				t.Fatalf("exact boundary rejected: plan=%v completion=%v", planErr, completionErr)
			}
			loadedPlan, err := services.plans.Get(ctx, plan.Hash)
			if err != nil || !bytes.Equal(loadedPlan.Payload, boundary.value) {
				t.Fatalf("exact plan round trip = %d bytes, %v", len(loadedPlan.Payload), err)
			}
			loadedRecord, err := services.idempotency.Lookup(ctx, record)
			if err != nil || loadedRecord == nil || !bytes.Equal(loadedRecord.Result.Data, boundary.value) {
				t.Fatalf("exact result round trip = %#v, %v", loadedRecord, err)
			}
		})
	}
}

type postgresJSONBoundary struct {
	name   string
	value  json.RawMessage
	within bool
}

func postgresJSONBoundaries() []postgresJSONBoundary {
	return []postgresJSONBoundary{
		{name: "bytes exact", value: json.RawMessage(`"` + strings.Repeat("x", int(action.MaxJSONDocumentBytes)-2) + `"`), within: true},
		{name: "bytes above", value: json.RawMessage(`"` + strings.Repeat("x", int(action.MaxJSONDocumentBytes)-1) + `"`)},
		{name: "depth exact", value: postgresNestedJSON(action.MaxJSONNestingDepth), within: true},
		{name: "depth above", value: postgresNestedJSON(action.MaxJSONNestingDepth + 1)},
		{name: "nodes exact", value: postgresArrayJSON(action.MaxJSONValueNodes - 1), within: true},
		{name: "nodes above", value: postgresArrayJSON(action.MaxJSONValueNodes)},
		{name: "number exact", value: postgresNumberJSON(action.MaxJSONNumberBytes), within: true},
		{name: "number above", value: postgresNumberJSON(action.MaxJSONNumberBytes + 1)},
	}
}

func postgresNestedJSON(depth int) json.RawMessage {
	return json.RawMessage(strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth))
}

func postgresArrayJSON(values int) json.RawMessage {
	return json.RawMessage("[" + strings.TrimSuffix(strings.Repeat("0,", values), ",") + "]")
}

func postgresNumberJSON(bytes int) json.RawMessage {
	return json.RawMessage("1" + strings.Repeat("0", bytes-1))
}

func TestPostgresStoresAcceptCanonicalMaximumPolicyFingerprint(t *testing.T) {
	services := startTestServices(t)
	ctx := context.Background()
	fingerprint := strings.Repeat("界", authz.MaxFingerprintRunes)

	plan := validPlan()
	plan.DecisionFingerprint = fingerprint
	if err := services.plans.Save(ctx, plan); err != nil {
		t.Fatal(err)
	}
	loadedPlan, err := services.plans.Get(ctx, plan.Hash)
	if err != nil || loadedPlan.DecisionFingerprint != fingerprint {
		t.Fatalf("maximum plan fingerprint round trip = %d runes, %v", len([]rune(loadedPlan.DecisionFingerprint)), err)
	}

	record := validReservation()
	record.DecisionFingerprint = fingerprint
	if existing, err := services.idempotency.Reserve(ctx, record); err != nil || existing != nil {
		t.Fatalf("maximum idempotency fingerprint reservation = %#v, %v", existing, err)
	}
	loadedRecord, err := services.idempotency.Lookup(ctx, record)
	if err != nil || loadedRecord == nil || loadedRecord.DecisionFingerprint != fingerprint {
		t.Fatalf("maximum idempotency fingerprint round trip = %#v, %v", loadedRecord, err)
	}
}

func TestPostgresPlanFingerprintFailsClosedAtAPIAndSchemaBoundaries(t *testing.T) {
	services := startTestServices(t)
	ctx := context.Background()
	plan := validPlan()
	plan.DecisionFingerprint = ""
	if err := services.plans.Save(ctx, plan); err == nil {
		t.Fatal("plan store accepted an empty decision fingerprint")
	}

	plan = validPlan()
	if err := services.plans.Save(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := services.db.Exec(
		`UPDATE modary_action_plan SET decision_fingerprint = '' WHERE plan_hash = $1`,
		plan.Hash,
	); err == nil {
		t.Fatal("PostgreSQL schema accepted an empty plan decision fingerprint")
	}
	loaded, err := services.plans.Get(ctx, plan.Hash)
	if err != nil || loaded.DecisionFingerprint != plan.DecisionFingerprint {
		t.Fatalf("failed schema write changed stored fingerprint: %#v, %v", loaded, err)
	}
}

func TestIdempotencyStorePreservesProvenanceAndResult(t *testing.T) {
	services := startTestServices(t)
	ctx := context.Background()
	reservation := validReservation()
	wantRunning := cloneRecordForTest(reservation)
	existing, err := services.idempotency.Reserve(ctx, reservation)
	if err != nil || existing != nil {
		t.Fatalf("first Reserve = %#v, %v", existing, err)
	}
	reservation.Impact.Resources[0] = "mutated"
	running, err := services.idempotency.Lookup(ctx, wantRunning)
	if err != nil || running == nil {
		t.Fatalf("running Lookup = %#v, %v", running, err)
	}
	if !reflect.DeepEqual(*running, wantRunning) {
		t.Fatalf("running record = %#v, want %#v", *running, wantRunning)
	}
	existing, err = services.idempotency.Reserve(ctx, wantRunning)
	if err != nil || existing == nil || !reflect.DeepEqual(*existing, wantRunning) {
		t.Fatalf("repeated Reserve = %#v, %v", existing, err)
	}

	conflict := cloneRecordForTest(wantRunning)
	conflict.ActionVersion = "9.9.9"
	conflict.ContractHash = digest('f')
	conflict.Channel = "http"
	existing, err = services.idempotency.Reserve(ctx, conflict)
	if err != nil || existing == nil || !reflect.DeepEqual(*existing, wantRunning) {
		t.Fatalf("conflicting Reserve did not return immutable binding: %#v, %v", existing, err)
	}
	typeIsolated := cloneRecordForTest(wantRunning)
	typeIsolated.ActorType = "service"
	existing, err = services.idempotency.Reserve(ctx, typeIsolated)
	if err != nil || existing != nil {
		t.Fatalf("actor type did not isolate idempotency identity: %#v, %v", existing, err)
	}
	isolated, err := services.idempotency.Lookup(ctx, typeIsolated)
	if err != nil || isolated == nil || !reflect.DeepEqual(*isolated, typeIsolated) {
		t.Fatalf("actor-type-isolated Lookup = %#v, %v", isolated, err)
	}

	completed := cloneRecordForTest(wantRunning)
	completed.Status = actionpersistence.IdempotencyCompleted
	completed.Result = action.Result{
		Data:    json.RawMessage(`{"resource_id":"item-42","written":true}`),
		Summary: "resource written",
		References: []audit.Reference{
			{Kind: "resource", ID: "item-42"},
			{Kind: "change", ID: "change-7"},
		},
	}
	wantCompleted := cloneRecordForTest(completed)
	if err := services.idempotency.Complete(ctx, completed); err != nil {
		t.Fatal(err)
	}
	completed.Result.Data[0] = '['
	completed.Result.References[0].ID = "mutated"
	stored, err := services.idempotency.Lookup(ctx, wantRunning)
	if err != nil || stored == nil {
		t.Fatalf("completed Lookup = %#v, %v", stored, err)
	}
	if !reflect.DeepEqual(*stored, wantCompleted) {
		t.Fatalf("completed record = %#v, want %#v", *stored, wantCompleted)
	}
	if err := services.idempotency.Complete(ctx, wantCompleted); err == nil {
		t.Fatal("second completion rewrote a completed record")
	}

	abortable := cloneRecordForTest(wantRunning)
	abortable.Key = "abort-me"
	if _, err := services.idempotency.Reserve(ctx, abortable); err != nil {
		t.Fatal(err)
	}
	if err := services.idempotency.Abort(ctx, abortable); err != nil {
		t.Fatal(err)
	}
	missing, err := services.idempotency.Lookup(ctx, abortable)
	if err != nil || missing != nil {
		t.Fatalf("aborted Lookup = %#v, %v", missing, err)
	}
}

func TestPlanAndIdempotencyRecordsSurviveRestartExactly(t *testing.T) {
	options := newTestOptions(t)
	first := startTestServicesWithOptions(t, options)
	plan := validPlan()
	reservation := validReservation()
	completed := cloneRecordForTest(reservation)
	completed.Status = actionpersistence.IdempotencyCompleted
	completed.Result = action.Result{
		Data:       json.RawMessage(`{"ok":true,"count":2}`),
		Summary:    "persisted",
		References: []audit.Reference{{Kind: "write", ID: "write-2"}},
	}
	if err := first.plans.Save(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if existing, err := first.idempotency.Reserve(context.Background(), reservation); err != nil || existing != nil {
		t.Fatalf("Reserve = %#v, %v", existing, err)
	}
	if err := first.idempotency.Complete(context.Background(), completed); err != nil {
		t.Fatal(err)
	}
	if err := first.host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	second := startTestServicesWithOptions(t, options)
	gotPlan, err := second.plans.Get(context.Background(), plan.Hash)
	if err != nil || !reflect.DeepEqual(gotPlan, plan) {
		t.Fatalf("restarted plan = %#v, %v", gotPlan, err)
	}
	gotRecord, err := second.idempotency.Lookup(context.Background(), reservation)
	if err != nil || gotRecord == nil || !reflect.DeepEqual(*gotRecord, completed) {
		t.Fatalf("restarted idempotency = %#v, %v", gotRecord, err)
	}
}

func TestTransactionManagerCommitsRollsBackAndNests(t *testing.T) {
	services := startTestServices(t)
	ctx := context.Background()
	if _, err := services.db.Exec(`CREATE TABLE transaction_probe (value TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	rollbackPlan := validPlan()
	rollbackRecord := validReservation()
	sentinel := errors.New("rollback requested")
	err := services.transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := services.plans.Save(txCtx, rollbackPlan); err != nil {
			return err
		}
		if existing, err := services.idempotency.Reserve(txCtx, rollbackRecord); err != nil || existing != nil {
			return errors.Join(err, errors.New("unexpected existing reservation"))
		}
		executor, err := services.control.Executor(txCtx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(txCtx,
			`INSERT INTO transaction_probe (value) VALUES ('rolled-back')`); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("rollback error = %v", err)
	}
	if _, err := services.plans.Get(ctx, rollbackPlan.Hash); !errors.Is(err, actionpersistence.ErrPlanNotFound) {
		t.Fatalf("rolled-back plan persisted: %v", err)
	}
	if record, err := services.idempotency.Lookup(ctx, rollbackRecord); err != nil || record != nil {
		t.Fatalf("rolled-back reservation = %#v, %v", record, err)
	}
	assertProbeCount(t, services, 0)
	panicked := false
	func() {
		defer func() { panicked = recover() != nil }()
		_ = services.transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
			executor, err := services.control.Executor(txCtx)
			if err != nil {
				return err
			}
			if _, err := executor.ExecContext(txCtx,
				`INSERT INTO transaction_probe (value) VALUES ('panic')`); err != nil {
				return err
			}
			panic("transaction callback panic")
		})
	}()
	if !panicked {
		t.Fatal("transaction callback panic did not propagate")
	}
	assertProbeCount(t, services, 0)

	if err := services.transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := services.plans.Save(txCtx, rollbackPlan); err != nil {
			return err
		}
		if existing, err := services.idempotency.Reserve(txCtx, rollbackRecord); err != nil || existing != nil {
			return errors.Join(err, errors.New("unexpected existing reservation"))
		}
		return services.transactions.WithinTransaction(txCtx, func(nested context.Context) error {
			executor, err := services.control.Executor(nested)
			if err != nil {
				return err
			}
			_, err = executor.ExecContext(nested,
				`INSERT INTO transaction_probe (value) VALUES ('committed')`)
			return err
		})
	}); err != nil {
		t.Fatal(err)
	}
	assertProbeCount(t, services, 1)

	completed := cloneRecordForTest(rollbackRecord)
	completed.Status = actionpersistence.IdempotencyCompleted
	completed.Result = action.Result{Data: json.RawMessage(`{"ok":true}`), Summary: "done"}
	err = services.transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := services.idempotency.Complete(txCtx, completed); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("completion rollback error = %v", err)
	}
	running, err := services.idempotency.Lookup(ctx, rollbackRecord)
	if err != nil || running == nil || running.Status != actionpersistence.IdempotencyRunning {
		t.Fatalf("completion escaped rollback: %#v, %v", running, err)
	}
}

func TestRootTransactionNilPanicRollsBackAndPropagates(t *testing.T) {
	services := startTestServices(t)
	ctx := context.Background()
	if _, err := services.db.Exec(`CREATE TABLE transaction_nil_panic_probe (value TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}

	returned := false
	var returnedErr error
	func() {
		defer func() {
			if !returned {
				_ = recover()
			}
		}()
		returnedErr = services.transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
			executor, err := services.control.Executor(txCtx)
			if err != nil {
				return err
			}
			if _, err := executor.ExecContext(txCtx,
				`INSERT INTO transaction_nil_panic_probe (value) VALUES ('rolled-back')`); err != nil {
				return err
			}
			panic(nil)
		})
		returned = true
	}()
	if returned {
		t.Fatalf("WithinTransaction returned instead of propagating panic(nil): %v", returnedErr)
	}
	var count int
	if err := services.db.QueryRow(`SELECT COUNT(*) FROM transaction_nil_panic_probe`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rows after panic(nil) = %d, want 0", count)
	}
}

func TestConcurrentIdempotencyReservationHasOneWinner(t *testing.T) {
	services := startTestServices(t)
	record := validReservation()
	const workers = 24
	type outcome struct {
		existing *actionpersistence.IdempotencyRecord
		err      error
	}
	ready := make(chan struct{})
	outcomes := make(chan outcome, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-ready
			existing, err := services.idempotency.Reserve(context.Background(), cloneRecordForTest(record))
			outcomes <- outcome{existing: existing, err: err}
		}()
	}
	close(ready)
	wait.Wait()
	close(outcomes)
	winners := 0
	for result := range outcomes {
		if result.err != nil {
			t.Fatalf("concurrent Reserve: %v", result.err)
		}
		if result.existing == nil {
			winners++
			continue
		}
		if !reflect.DeepEqual(*result.existing, record) {
			t.Fatalf("concurrent existing record = %#v", *result.existing)
		}
	}
	if winners != 1 {
		t.Fatalf("reservation winners = %d, want 1", winners)
	}
	var count int
	if err := services.db.QueryRow(`SELECT COUNT(*) FROM modary_action_idempotency`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("persisted reservations = %d", count)
	}
}

func TestCorruptedRowsFailClosed(t *testing.T) {
	services := startTestServices(t)
	plan := validPlan()
	if err := services.plans.Save(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if _, err := services.db.Exec(`UPDATE modary_action_plan SET impact_resources_json = '[1]' WHERE plan_hash = $1`, plan.Hash); err != nil {
		t.Fatal(err)
	}
	if _, err := services.plans.Get(context.Background(), plan.Hash); err == nil {
		t.Fatal("plan store accepted corrupted impact resources")
	}

	record := validReservation()
	if _, err := services.idempotency.Reserve(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	completed := cloneRecordForTest(record)
	completed.Status = actionpersistence.IdempotencyCompleted
	completed.Result = action.Result{Data: json.RawMessage(`{}`)}
	if err := services.idempotency.Complete(context.Background(), completed); err != nil {
		t.Fatal(err)
	}
	if _, err := services.db.Exec(`
		UPDATE modary_action_idempotency
		SET result_references_json = '[{"kind":"write","id":"one"},{"kind":"write","id":"one"}]'
		WHERE scope_kind = $1 AND scope_id = $2 AND actor_id = $3 AND actor_type = $4
		  AND action_id = $5 AND idempotency_key = $6`,
		record.Scope.Kind, record.Scope.ID, record.ActorID, record.ActorType, record.ActionID, record.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := services.idempotency.Lookup(context.Background(), record); err == nil || !strings.Contains(err.Error(), "reference is duplicated") {
		t.Fatalf("corrupted reference error = %v", err)
	}
}

func TestStoredActionJSONIsBoundedBeforePostgreSQLScan(t *testing.T) {
	services := startTestServices(t)
	ctx := context.Background()
	plan := validPlan()
	if err := services.plans.Save(ctx, plan); err != nil {
		t.Fatal(err)
	}
	record := validReservation()
	if _, err := services.idempotency.Reserve(ctx, record); err != nil {
		t.Fatal(err)
	}
	completed := cloneRecordForTest(record)
	completed.Status = actionpersistence.IdempotencyCompleted
	completed.Result = action.Result{Data: json.RawMessage(`{}`)}
	if err := services.idempotency.Complete(ctx, completed); err != nil {
		t.Fatal(err)
	}

	oversized := []byte(`"` + strings.Repeat("x", int(action.MaxJSONDocumentBytes)) + `"`)
	oversizedMultibyte := []byte(`"` + strings.Repeat("界", int(action.MaxJSONDocumentBytes/3)+1) + `"`)
	oversizedCollection := []byte(`["` + strings.Repeat("界", int(action.MaxJSONDocumentBytes/3)+1) + `"]`)
	if int64(len(oversizedMultibyte)) <= action.MaxJSONDocumentBytes || int64(len(oversizedCollection)) <= action.MaxJSONDocumentBytes {
		t.Fatalf("oversized multibyte fixtures = %d and %d bytes", len(oversizedMultibyte), len(oversizedCollection))
	}
	if _, err := services.db.ExecContext(ctx, `UPDATE modary_action_plan SET payload_json = $1 WHERE plan_hash = $2`, oversized, plan.Hash); err == nil {
		t.Fatal("PostgreSQL schema accepted an oversized plan payload")
	}
	if _, err := services.db.ExecContext(ctx, `UPDATE modary_action_plan SET impact_resources_json = $1 WHERE plan_hash = $2`, string(oversizedCollection), plan.Hash); err == nil {
		t.Fatal("PostgreSQL schema accepted oversized plan impact resources")
	}
	if _, err := services.db.ExecContext(ctx, `UPDATE modary_action_plan SET payload_json = $1 WHERE plan_hash = $2`, oversizedMultibyte, plan.Hash); err == nil {
		t.Fatal("PostgreSQL schema counted a multibyte plan payload by characters instead of bytes")
	}
	if _, err := services.db.ExecContext(ctx, `
		UPDATE modary_action_idempotency SET impact_resources_json = $1
		WHERE scope_kind = $2 AND scope_id = $3 AND actor_id = $4 AND actor_type = $5
		  AND action_id = $6 AND idempotency_key = $7`,
		string(oversizedCollection), record.Scope.Kind, record.Scope.ID, record.ActorID, record.ActorType, record.ActionID, record.Key); err == nil {
		t.Fatal("PostgreSQL schema accepted oversized idempotency impact resources")
	}
	if _, err := services.db.ExecContext(ctx, `
		UPDATE modary_action_idempotency SET result_references_json = $1
		WHERE scope_kind = $2 AND scope_id = $3 AND actor_id = $4 AND actor_type = $5
		  AND action_id = $6 AND idempotency_key = $7`,
		string(oversizedCollection), record.Scope.Kind, record.Scope.ID, record.ActorID, record.ActorType, record.ActionID, record.Key); err == nil {
		t.Fatal("PostgreSQL schema accepted oversized idempotency references")
	}
	if _, err := services.db.ExecContext(ctx, `
		UPDATE modary_action_idempotency SET result_data_json = $1
		WHERE scope_kind = $2 AND scope_id = $3 AND actor_id = $4 AND actor_type = $5
		  AND action_id = $6 AND idempotency_key = $7`,
		oversizedMultibyte, record.Scope.Kind, record.Scope.ID, record.ActorID, record.ActorType, record.ActionID, record.Key); err == nil {
		t.Fatal("PostgreSQL schema counted a multibyte idempotency result by characters instead of bytes")
	}
	connection, err := services.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := connection.ExecContext(ctx, `
		ALTER TABLE modary_action_plan
			DROP CONSTRAINT modary_action_plan_payload_json_check,
			DROP CONSTRAINT modary_action_plan_impact_resources_json_check,
			DROP CONSTRAINT modary_action_plan_actor_id_check;
		ALTER TABLE modary_action_idempotency
			DROP CONSTRAINT modary_action_idempotency_impact_resources_json_check,
			DROP CONSTRAINT modary_action_idempotency_result_references_json_check,
			DROP CONSTRAINT modary_action_idempotency_result_data_json_check,
			DROP CONSTRAINT modary_action_idempotency_result_summary_check`); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.ExecContext(ctx, `UPDATE modary_action_plan SET impact_resources_json = $1 WHERE plan_hash = $2`, string(oversizedCollection), plan.Hash); err != nil {
		t.Fatal(err)
	}
	if _, err := services.plans.Get(ctx, plan.Hash); err == nil || !strings.Contains(err.Error(), "impact resources exceed the JSON resource limit") {
		t.Fatalf("oversized stored plan impact resources error = %v", err)
	}
	planResources, err := json.Marshal(plan.Impact.Resources)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.ExecContext(ctx, `UPDATE modary_action_plan SET impact_resources_json = $1 WHERE plan_hash = $2`, string(planResources), plan.Hash); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.ExecContext(ctx, `
		UPDATE modary_action_idempotency SET impact_resources_json = $1
		WHERE scope_kind = $2 AND scope_id = $3 AND actor_id = $4 AND actor_type = $5
		  AND action_id = $6 AND idempotency_key = $7`,
		string(oversizedCollection), record.Scope.Kind, record.Scope.ID, record.ActorID, record.ActorType, record.ActionID, record.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := services.idempotency.Lookup(ctx, record); err == nil || !strings.Contains(err.Error(), "impact resources exceed the JSON resource limit") {
		t.Fatalf("oversized stored idempotency impact resources error = %v", err)
	}
	recordResources, err := json.Marshal(record.Impact.Resources)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.ExecContext(ctx, `
		UPDATE modary_action_idempotency SET impact_resources_json = $1
		WHERE scope_kind = $2 AND scope_id = $3 AND actor_id = $4 AND actor_type = $5
		  AND action_id = $6 AND idempotency_key = $7`,
		string(recordResources), record.Scope.Kind, record.Scope.ID, record.ActorID, record.ActorType, record.ActionID, record.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.ExecContext(ctx, `
		UPDATE modary_action_idempotency SET result_references_json = $1
		WHERE scope_kind = $2 AND scope_id = $3 AND actor_id = $4 AND actor_type = $5
		  AND action_id = $6 AND idempotency_key = $7`,
		string(oversizedCollection), record.Scope.Kind, record.Scope.ID, record.ActorID, record.ActorType, record.ActionID, record.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := services.idempotency.Lookup(ctx, record); err == nil || !strings.Contains(err.Error(), "references exceed the JSON resource limit") {
		t.Fatalf("oversized stored idempotency references error = %v", err)
	}
	if _, err := connection.ExecContext(ctx, `
		UPDATE modary_action_idempotency SET result_references_json = '[]'
		WHERE scope_kind = $1 AND scope_id = $2 AND actor_id = $3 AND actor_type = $4
		  AND action_id = $5 AND idempotency_key = $6`,
		record.Scope.Kind, record.Scope.ID, record.ActorID, record.ActorType, record.ActionID, record.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.ExecContext(ctx, `UPDATE modary_action_plan SET payload_json = $1 WHERE plan_hash = $2`, oversized, plan.Hash); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.ExecContext(ctx, `
		UPDATE modary_action_idempotency SET result_data_json = $1
		WHERE scope_kind = $2 AND scope_id = $3 AND actor_id = $4 AND actor_type = $5
		  AND action_id = $6 AND idempotency_key = $7`,
		oversized, record.Scope.Kind, record.Scope.ID, record.ActorID, record.ActorType, record.ActionID, record.Key); err != nil {
		t.Fatal(err)
	}

	if _, err := services.plans.Get(ctx, plan.Hash); err == nil || !strings.Contains(err.Error(), "resource limit") {
		t.Fatalf("oversized stored plan error = %v", err)
	}
	if _, err := services.idempotency.Lookup(ctx, record); err == nil || !strings.Contains(err.Error(), "resource limit") {
		t.Fatalf("oversized stored result error = %v", err)
	}

	if _, err := connection.ExecContext(ctx, `UPDATE modary_action_plan SET payload_json = $1, actor_id = $2 WHERE plan_hash = $3`,
		[]byte(plan.Payload), strings.Repeat("界", maxStoredActorIDBytes/3+1), plan.Hash); err != nil {
		t.Fatal(err)
	}
	if _, err := services.plans.Get(ctx, plan.Hash); err == nil || !strings.Contains(err.Error(), "actor id") || !strings.Contains(err.Error(), "resource limit") {
		t.Fatalf("oversized stored plan scalar error = %v", err)
	}
	if _, err := connection.ExecContext(ctx, `
		UPDATE modary_action_idempotency
		SET result_data_json = $1, result_summary = $2
		WHERE scope_kind = $3 AND scope_id = $4 AND actor_id = $5 AND actor_type = $6
		  AND action_id = $7 AND idempotency_key = $8`,
		[]byte(`{}`), strings.Repeat("界", maxStoredSummaryBytes/3+1),
		record.Scope.Kind, record.Scope.ID, record.ActorID, record.ActorType, record.ActionID, record.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := services.idempotency.Lookup(ctx, record); err == nil || !strings.Contains(err.Error(), "result summary") || !strings.Contains(err.Error(), "resource limit") {
		t.Fatalf("oversized stored idempotency scalar error = %v", err)
	}
}

func assertProbeCount(t *testing.T, services testServices, want int) {
	t.Helper()
	var count int
	if err := services.db.QueryRow(`SELECT COUNT(*) FROM transaction_probe`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("transaction probe count = %d, want %d", count, want)
	}
}

func validPlan() action.Plan {
	createdAt := time.Date(2026, 7, 30, 12, 34, 56, 123456789, time.UTC)
	return action.Plan{
		Hash:                digest('a'),
		ActionID:            "resource.write",
		ActionVersion:       "1.2.3-beta.1+build.7",
		ContractHash:        digest('b'),
		ActorID:             "person-123",
		ActorType:           "human",
		Channel:             action.ChannelMCP,
		Scope:               scope.Must("account", "account/alpha"),
		InputHash:           digest('c'),
		Payload:             json.RawMessage(`{"name":"alpha","enabled":true}`),
		Impact:              authz.Impact{Rows: 2, Resources: []string{"resource/item-42", "index/items"}},
		SnapshotHash:        digest('d'),
		DecisionFingerprint: digest('e'),
		CreatedAt:           createdAt,
		ExpiresAt:           createdAt.Add(15 * time.Minute),
	}
}

func validReservation() actionpersistence.IdempotencyRecord {
	plan := validPlan()
	return actionpersistence.IdempotencyRecord{
		Scope:               plan.Scope,
		ActorID:             plan.ActorID,
		ActorType:           plan.ActorType,
		ActionID:            plan.ActionID,
		ActionVersion:       plan.ActionVersion,
		ContractHash:        plan.ContractHash,
		Channel:             plan.Channel,
		Key:                 "request-123",
		InputHash:           plan.InputHash,
		PlanHash:            plan.Hash,
		Impact:              authz.Impact{Rows: plan.Impact.Rows, Resources: append([]string(nil), plan.Impact.Resources...)},
		DecisionFingerprint: plan.DecisionFingerprint,
		Status:              actionpersistence.IdempotencyRunning,
	}
}

func digest(character byte) string {
	return "sha256:" + strings.Repeat(string(character), 64)
}

func clonePlanForTest(plan action.Plan) action.Plan {
	plan.Payload = append(json.RawMessage(nil), plan.Payload...)
	plan.Impact.Resources = append([]string(nil), plan.Impact.Resources...)
	return plan
}

func cloneRecordForTest(record actionpersistence.IdempotencyRecord) actionpersistence.IdempotencyRecord {
	record.Impact.Resources = append([]string(nil), record.Impact.Resources...)
	record.Result.Data = append(json.RawMessage(nil), record.Result.Data...)
	record.Result.References = append([]audit.Reference(nil), record.Result.References...)
	return record
}
