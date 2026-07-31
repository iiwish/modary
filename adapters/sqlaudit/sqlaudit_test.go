package sqlaudit

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/adapters/internal/sqlitetest"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/scope"
	_ "modernc.org/sqlite"
)

func TestEmptyInstallationCreatesNoEvents(t *testing.T) {
	db, _ := openHook(t)
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM modary_audit_event`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("audit event count = %d", count)
	}
	registration := Module(Options{})
	if registration.Definition.Manifest.ID != ModuleID || registration.Start == nil || len(registration.Definition.Migrations) != 1 || len(registration.Definition.Actions) != 0 {
		t.Fatalf("registration = %#v", registration.Definition)
	}
}

func TestRecordPersistsNormalizedStructuredProvenance(t *testing.T) {
	db, store := openHook(t)
	now := time.Date(2026, 7, 30, 8, 0, 0, 123, time.FixedZone("test", 8*60*60))
	event := completeEvent(now)
	event.ResultSummary = strings.Repeat("界", audit.MaxSummaryRunes+20)
	event.Reason = strings.Repeat("r", audit.MaxReasonRunes+20)
	if err := store.Record(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.load(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RequestID != event.RequestID || loaded.ActorID != event.ActorID || loaded.ActorType != event.ActorType ||
		loaded.Channel != event.Channel || loaded.ActionID != event.ActionID || loaded.ActionVersion != event.ActionVersion ||
		loaded.ContractHash != event.ContractHash || loaded.Scope != event.Scope || loaded.InputHash != event.InputHash ||
		loaded.PlanHash != event.PlanHash || loaded.Decision != event.Decision || loaded.AuditLevel != event.AuditLevel {
		t.Fatalf("loaded provenance = %#v", loaded)
	}
	if utf8.RuneCountInString(loaded.ResultSummary) != audit.MaxSummaryRunes || utf8.RuneCountInString(loaded.Reason) != audit.MaxReasonRunes {
		t.Fatalf("normalized text lengths = %d, %d", utf8.RuneCountInString(loaded.ResultSummary), utf8.RuneCountInString(loaded.Reason))
	}
	if loaded.Impact == nil || loaded.Impact.Rows != 7 || len(loaded.Impact.Resources) != 2 ||
		len(loaded.ResultRefs) != 1 || loaded.ResultRefs[0] != (audit.Reference{Kind: "counter", ID: "counter-1"}) {
		t.Fatalf("loaded structured data = %#v", loaded)
	}
	if loaded.StartedAt.Location() != time.UTC || !loaded.StartedAt.Equal(event.StartedAt) || !loaded.FinishedAt.Equal(event.FinishedAt) {
		t.Fatalf("loaded times = %s, %s", loaded.StartedAt, loaded.FinishedAt)
	}

	var scopeKind, scopeID, contractHash, referencesJSON string
	if err := db.QueryRow(`SELECT scope_kind, scope_id, contract_hash, result_references_json FROM modary_audit_event WHERE event_id = 1`).Scan(
		&scopeKind, &scopeID, &contractHash, &referencesJSON); err != nil {
		t.Fatal(err)
	}
	if scopeKind != "account" || scopeID != "account-1" || !strings.HasPrefix(contractHash, "sha256:") || !strings.Contains(referencesJSON, "counter-1") {
		t.Fatalf("stored provenance = %q, %q, %q, %q", scopeKind, scopeID, contractHash, referencesJSON)
	}
}

func TestMetadataAuditDropsImpactAndReferences(t *testing.T) {
	_, store := openHook(t)
	event := completeEvent(time.Now())
	event.AuditLevel = "metadata"
	if err := store.Record(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.load(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Impact != nil || len(loaded.ResultRefs) != 0 {
		t.Fatalf("metadata event = %#v", loaded)
	}
}

func TestFailureAuditPersistsNormalizedKindAndOutcome(t *testing.T) {
	_, store := openHook(t)
	event := completeEvent(time.Now())
	event.Decision = "rejected"
	event.ErrorCode = "COUNTER.NOT_READY"
	event.ErrorKind = string(action.ErrorKindPrecondition)
	event.Reason = "counter is not ready"
	if err := store.Record(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.load(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Decision != event.Decision || loaded.ErrorCode != event.ErrorCode || loaded.ErrorKind != event.ErrorKind || loaded.Reason != event.Reason {
		t.Fatalf("loaded failure audit = %#v", loaded)
	}
}

func TestAuditParticipatesInCallerTransaction(t *testing.T) {
	db, store := openHook(t)
	rollback := errors.New("rollback audit")
	err := store.control.WithinTransaction(context.Background(), func(ctx context.Context) error {
		if err := store.Record(ctx, completeEvent(time.Now())); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("transaction error = %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM modary_audit_event`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rolled-back audit count = %d, %v", count, err)
	}

	if err := store.control.WithinTransaction(context.Background(), func(ctx context.Context) error {
		return store.Record(ctx, completeEvent(time.Now()))
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM modary_audit_event`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("committed audit count = %d, %v", count, err)
	}
}

func TestInvalidAndPartialEventsFailClosed(t *testing.T) {
	_, store := openHook(t)
	base := completeEvent(time.Now())
	tests := []struct {
		name   string
		mutate func(*audit.Event)
	}{
		{name: "missing request", mutate: func(event *audit.Event) { event.RequestID = "" }},
		{name: "missing action", mutate: func(event *audit.Event) { event.ActionID = "" }},
		{name: "invalid decision", mutate: func(event *audit.Event) { event.Decision = "maybe" }},
		{name: "partial scope", mutate: func(event *audit.Event) { event.Scope.ID = "" }},
		{name: "zero start", mutate: func(event *audit.Event) { event.StartedAt = time.Time{} }},
		{name: "reverse time", mutate: func(event *audit.Event) { event.FinishedAt = event.StartedAt.Add(-time.Second) }},
		{name: "timestamp outside storage range", mutate: func(event *audit.Event) {
			event.StartedAt = time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)
			event.FinishedAt = event.StartedAt.Add(time.Second)
		}},
		{name: "duplicate resource", mutate: func(event *audit.Event) { event.Impact.Resources = []string{"counter", "counter"} }},
		{name: "missing success actor", mutate: func(event *audit.Event) { event.ActorID = "" }},
		{name: "missing success actor type", mutate: func(event *audit.Event) { event.ActorType = "" }},
		{name: "missing success channel", mutate: func(event *audit.Event) { event.Channel = "" }},
		{name: "noncanonical success action", mutate: func(event *audit.Event) { event.ActionID = "counter/increment" }},
		{name: "missing action version", mutate: func(event *audit.Event) { event.ActionVersion = "" }},
		{name: "invalid action version", mutate: func(event *audit.Event) { event.ActionVersion = "01.2.3" }},
		{name: "missing contract hash", mutate: func(event *audit.Event) { event.ContractHash = "" }},
		{name: "invalid input hash", mutate: func(event *audit.Event) { event.InputHash = "sha256:short" }},
		{name: "missing plan hash", mutate: func(event *audit.Event) { event.PlanHash = "" }},
		{name: "success error code", mutate: func(event *audit.Event) { event.ErrorCode = "INTERNAL" }},
		{name: "success error kind", mutate: func(event *audit.Event) { event.ErrorKind = string(action.ErrorKindInternal) }},
		{name: "detailed success without impact", mutate: func(event *audit.Event) { event.Impact = nil }},
		{name: "duplicate reference", mutate: func(event *audit.Event) {
			event.ResultRefs = append(event.ResultRefs, event.ResultRefs[0])
		}},
		{name: "decision laundering", mutate: func(event *audit.Event) { event.Decision = "all\nowed" }},
		{name: "level laundering", mutate: func(event *audit.Event) { event.AuditLevel = "detail\ned" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := base
			impact := *base.Impact
			impact.Resources = append([]string(nil), base.Impact.Resources...)
			event.Impact = &impact
			event.ResultRefs = append([]audit.Reference(nil), base.ResultRefs...)
			test.mutate(&event)
			if err := store.Record(context.Background(), event); err == nil {
				t.Fatal("invalid event was persisted")
			}
		})
	}
	failures := []audit.Event{
		{RequestID: "missing-code", ActionID: "invalid.request", Decision: "denied", ErrorKind: string(action.ErrorKindDenied), AuditLevel: "metadata", StartedAt: base.StartedAt, FinishedAt: base.FinishedAt},
		{RequestID: "missing-kind", ActionID: "invalid.request", Decision: "denied", ErrorCode: action.CodeAuthzDenied, AuditLevel: "metadata", StartedAt: base.StartedAt, FinishedAt: base.FinishedAt},
		{RequestID: "wrong-denied-kind", ActionID: "invalid.request", Decision: "denied", ErrorCode: action.CodeAuthzDenied, ErrorKind: string(action.ErrorKindValidation), AuditLevel: "metadata", StartedAt: base.StartedAt, FinishedAt: base.FinishedAt},
		{RequestID: "wrong-rejected-kind", ActionID: "invalid.request", Decision: "rejected", ErrorCode: action.CodeInternal, ErrorKind: string(action.ErrorKindInternal), AuditLevel: "metadata", StartedAt: base.StartedAt, FinishedAt: base.FinishedAt},
		{RequestID: "wrong-failed-kind", ActionID: "invalid.request", Decision: "failed", ErrorCode: action.CodeValidationFailed, ErrorKind: string(action.ErrorKindValidation), AuditLevel: "metadata", StartedAt: base.StartedAt, FinishedAt: base.FinishedAt},
		{RequestID: "mismatched-builtin", ActionID: "invalid.request", Decision: "failed", ErrorCode: action.CodeUnavailable, ErrorKind: string(action.ErrorKindInternal), AuditLevel: "metadata", StartedAt: base.StartedAt, FinishedAt: base.FinishedAt},
		{RequestID: "malformed-custom-code", ActionID: "invalid.request", Decision: "rejected", ErrorCode: "not-qualified", ErrorKind: string(action.ErrorKindConflict), AuditLevel: "metadata", StartedAt: base.StartedAt, FinishedAt: base.FinishedAt},
		{RequestID: "custom-internal", ActionID: "invalid.request", Decision: "failed", ErrorCode: "COUNTER.PRIVATE", ErrorKind: string(action.ErrorKindInternal), AuditLevel: "metadata", StartedAt: base.StartedAt, FinishedAt: base.FinishedAt},
	}
	for _, event := range failures {
		if err := store.Record(context.Background(), event); err == nil {
			t.Fatalf("invalid failure audit was persisted: %#v", event)
		}
	}
	if err := store.Record(context.Background(), audit.Event{
		RequestID: "request-invalid", ActionID: "invalid.request", Decision: "denied", AuditLevel: "metadata",
		ErrorCode: action.CodeAuthzDenied, ErrorKind: string(action.ErrorKindDenied),
		StartedAt: base.StartedAt, FinishedAt: base.FinishedAt,
	}); err != nil {
		t.Fatalf("zero scope validation audit was rejected: %v", err)
	}
	sanitized := base
	sanitized.RequestID = "request-sanitized"
	sanitized.ResultRefs = []audit.Reference{{Kind: "counter", ID: ""}}
	if err := store.Record(context.Background(), sanitized); err != nil {
		t.Fatalf("normalizable reference was rejected: %v", err)
	}
	loaded, err := store.load(context.Background(), 2)
	if err != nil || len(loaded.ResultRefs) != 0 {
		t.Fatalf("sanitized references = %#v, %v", loaded.ResultRefs, err)
	}
	freeText := base
	freeText.RequestID = "request-free-text"
	freeText.ResultSummary = "line one\nline two\x00" + string([]byte{0xff})
	freeText.Reason = "reason\r\nnext"
	if err := store.Record(context.Background(), freeText); err != nil {
		t.Fatalf("normalizable free text was rejected: %v", err)
	}
	loaded, err = store.load(context.Background(), 3)
	if err != nil || !utf8.ValidString(loaded.ResultSummary) || strings.ContainsAny(loaded.ResultSummary, "\r\n\x00") || strings.ContainsAny(loaded.Reason, "\r\n\x00") {
		t.Fatalf("normalized free text = %q, %q, %v", loaded.ResultSummary, loaded.Reason, err)
	}
}

func TestCorruptRowsAreRejectedOnRead(t *testing.T) {
	db, store := openHook(t)
	if err := store.Record(context.Background(), completeEvent(time.Now())); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE modary_audit_event SET result_references_json = '[{"kind":"counter","id":"one","unknown":true}]' WHERE event_id = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.load(context.Background(), 1); err == nil {
		t.Fatal("corrupt reference JSON was accepted")
	}
	if _, err := db.Exec(`UPDATE modary_audit_event SET result_references_json = '[]', finished_at = 'not-a-time' WHERE event_id = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.load(context.Background(), 1); err == nil {
		t.Fatal("corrupt timestamp was accepted")
	}
	if _, err := db.Exec(`UPDATE modary_audit_event SET finished_at = started_at, started_at = '2026-07-30T08:00:00Z' WHERE event_id = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.load(context.Background(), 1); err == nil {
		t.Fatal("parseable noncanonical timestamp was accepted")
	}
	canonical := formatTimestamp(time.Now())
	if _, err := db.Exec(`UPDATE modary_audit_event SET started_at = ?, finished_at = ?, contract_hash = 'sha256:short' WHERE event_id = 1`, canonical, canonical); err != nil {
		t.Fatal(err)
	}
	if _, err := store.load(context.Background(), 1); err == nil {
		t.Fatal("corrupt success provenance was accepted")
	}
	if _, err := db.Exec(`UPDATE modary_audit_event SET contract_hash = ?, result_references_json = '[{"kind":"counter","id":"one"},{"kind":"counter","id":"one"}]' WHERE event_id = 1`, "sha256:"+strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.load(context.Background(), 1); err == nil {
		t.Fatal("duplicate stored references were accepted")
	}
	if _, err := db.Exec(`UPDATE modary_audit_event
		SET started_at = ?, finished_at = ?, result_references_json = '[]',
		    audit_level = 'metadata', impact_rows = 1, impact_resources_json = '[]'
		WHERE event_id = 1`, canonical, canonical); err != nil {
		t.Fatal(err)
	}
	if _, err := store.load(context.Background(), 1); err == nil {
		t.Fatal("metadata event with stored impact was accepted")
	}
}

func TestConcurrentRecord(t *testing.T) {
	db, store := openHook(t)
	const workers = 24
	var group sync.WaitGroup
	for index := range workers {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			event := completeEvent(time.Now())
			event.RequestID = "request-" + strings.Repeat("x", index%8) + string(rune('a'+index))
			if err := store.Record(context.Background(), event); err != nil {
				t.Errorf("Record(%d) error = %v", index, err)
			}
		}(index)
	}
	group.Wait()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM modary_audit_event`).Scan(&count); err != nil || count != workers {
		t.Fatalf("concurrent event count = %d, %v", count, err)
	}
}

func TestHookRejectsNilContextAndUnavailableStore(t *testing.T) {
	if err := (&hook{}).Record(nil, audit.Event{}); !errors.Is(err, ErrContextRequired) {
		t.Fatalf("Record(nil) error = %v", err)
	}
	if err := (&hook{}).Record(context.Background(), completeEvent(time.Now())); err == nil {
		t.Fatal("unavailable Hook succeeded")
	}
	if _, err := (&hook{}).load(nil, 1); !errors.Is(err, ErrContextRequired) {
		t.Fatalf("load(nil) error = %v", err)
	}
}

func openHook(t *testing.T) (*sql.DB, *hook) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "audit.db")+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(8)
	t.Cleanup(func() { _ = db.Close() })
	control, err := sqlitetest.NewControl(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := control.ApplyMigrations(context.Background(), ModuleID, sqliteMigrations); err != nil {
		t.Fatal(err)
	}
	return db, &hook{control: control}
}

func completeEvent(now time.Time) audit.Event {
	now = now.UTC()
	return audit.Event{
		RequestID:     "request-1",
		ActorID:       "person-one",
		ActorType:     "human",
		Channel:       "http",
		ActionID:      "counter.increment",
		ActionVersion: "1.2.3",
		ContractHash:  "sha256:" + strings.Repeat("a", 64),
		Scope:         scope.Must("account", "account-1"),
		InputHash:     "sha256:" + strings.Repeat("b", 64),
		PlanHash:      "sha256:" + strings.Repeat("c", 64),
		Decision:      "allowed",
		AuditLevel:    "detailed",
		ResultSummary: "incremented counter",
		Impact:        &audit.Impact{Rows: 7, Resources: []string{"counter", "counter-history"}},
		ResultRefs:    []audit.Reference{{Kind: "counter", ID: "counter-1"}},
		StartedAt:     now,
		FinishedAt:    now.Add(time.Second),
	}
}
