package action_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/scope"
)

func TestErrorKindIsClosedAndBuiltinCodesHaveStableKinds(t *testing.T) {
	tests := []struct {
		code string
		kind action.ErrorKind
	}{
		{action.CodeActionNotFound, action.ErrorKindNotFound},
		{action.CodeValidationFailed, action.ErrorKindValidation},
		{action.CodeAuthzDenied, action.ErrorKindDenied},
		{action.CodePreconditionFailed, action.ErrorKindPrecondition},
		{action.CodePlanRequired, action.ErrorKindPreconditionRequired},
		{action.CodePlanNotFound, action.ErrorKindNotFound},
		{action.CodePlanStale, action.ErrorKindConflict},
		{action.CodeLimitExceeded, action.ErrorKindLimit},
		{action.CodeIdempotencyRequired, action.ErrorKindPreconditionRequired},
		{action.CodeIdempotencyConflict, action.ErrorKindConflict},
		{action.CodeIdempotencyProgress, action.ErrorKindConflict},
		{action.CodeUnavailable, action.ErrorKindUnavailable},
		{action.CodeInternal, action.ErrorKindInternal},
	}
	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			kind, ok := action.BuiltinErrorKind(test.code)
			if !ok || kind != test.kind {
				t.Fatalf("BuiltinErrorKind(%q) = %q, %v; want %q, true", test.code, kind, ok, test.kind)
			}
			if !kind.Valid() {
				t.Fatalf("built-in kind %q is not valid", kind)
			}
		})
	}
	if kind, ok := action.BuiltinErrorKind("INVENTORY.OUT_OF_STOCK"); ok || kind != "" {
		t.Fatalf("custom code classified as built-in: %q, %v", kind, ok)
	}
	if action.ErrorKind("").Valid() || action.ErrorKind("future_kind").Valid() {
		t.Fatal("ErrorKind.Valid accepted a value outside the closed set")
	}
}

func TestNewErrorAndWithRequestCarryStableKinds(t *testing.T) {
	err := action.NewError(action.CodePlanStale, "state changed")
	if err.Kind != action.ErrorKindConflict || action.ErrorKindOf(err) != action.ErrorKindConflict || !action.IsKind(err, action.ErrorKindConflict) {
		t.Fatalf("built-in error kind = %q / %q", err.Kind, action.ErrorKindOf(err))
	}

	legacy := &action.Error{Code: action.CodeValidationFailed, Message: "invalid"}
	wrapped := action.WithRequest(legacy, action.Request{}, "")
	var projected *action.Error
	if !errors.As(wrapped, &projected) || projected.Kind != action.ErrorKindValidation {
		t.Fatalf("WithRequest() kind = %#v", projected)
	}

	unknown := action.NewError("COUNTER.CUSTOM", "custom")
	if unknown.Kind != "" || action.ErrorKindOf(unknown) != action.ErrorKindInternal {
		t.Fatalf("undeclared custom error kind = %q / %q", unknown.Kind, action.ErrorKindOf(unknown))
	}
}

func TestWithRequestIsNilPreservingAndDoesNotDuplicateGovernedErrors(t *testing.T) {
	if got := action.WithRequest(nil, action.Request{}, ""); got != nil {
		t.Fatalf("WithRequest(nil) = %v", got)
	}
	original := action.NewError(action.CodePlanStale, "state changed")
	wrapped := action.WithRequest(original, action.Request{ActionID: "counter.increment"}, "counter.increment")
	var projected *action.Error
	if !errors.As(wrapped, &projected) || projected == nil || projected == original {
		t.Fatalf("WithRequest() projection = %#v", projected)
	}
	if projected.Cause != original.Cause {
		t.Fatal("WithRequest introduced a second governed Error through Cause")
	}
	if projected.ActionID != "counter.increment" || projected.RequiredPermission != "counter.increment" {
		t.Fatalf("WithRequest() context = %#v", projected)
	}

	invalidContext := action.WithRequest(original, action.Request{ActionID: strings.Repeat("x", 1000)}, "not valid")
	if !errors.As(invalidContext, &projected) || projected.ActionID != "" || projected.RequiredPermission != "" {
		t.Fatalf("invalid request context escaped: %#v", projected)
	}
}

func TestWithRequestReplacesAndSanitizesAllRequestContext(t *testing.T) {
	executionScope := scope.Must("tenant", "tenant-a")
	first := action.WithRequest(action.NewError(action.CodePreconditionFailed, "not ready"), action.Request{
		RequestID: "request-a",
		Actor:     identity.Actor{ID: "actor-a"},
		ActionID:  "counter.increment",
		Scope:     executionScope,
	}, "counter.increment")
	var firstError *action.Error
	if !errors.As(first, &firstError) || firstError.ActorID != "actor-a" || firstError.RequestID != "request-a" ||
		firstError.Scope == nil || *firstError.Scope != executionScope {
		t.Fatalf("first request context = %#v", firstError)
	}

	second := action.WithRequest(first, action.Request{
		RequestID: strings.Repeat("r", 129),
		Actor:     identity.Actor{ID: " actor-b "},
		ActionID:  strings.Repeat("x", 1000),
		Scope:     scope.Execution{Kind: "Invalid", ID: "tenant-b"},
	}, "not valid")
	var secondError *action.Error
	if !errors.As(second, &secondError) {
		t.Fatalf("second request error = %v", second)
	}
	if secondError.ActionID != "" || secondError.RequiredPermission != "" || secondError.ActorID != "" ||
		secondError.Scope != nil || secondError.RequestID != "" {
		t.Fatalf("invalid request context escaped or retained prior values: %#v", secondError)
	}
	if firstError.ActorID != "actor-a" || firstError.RequestID != "request-a" ||
		firstError.Scope == nil || *firstError.Scope != executionScope {
		t.Fatalf("second enrichment mutated the first error: %#v", firstError)
	}
	lineSeparated := action.WithRequest(first, action.Request{RequestID: "request\u2028other"}, "")
	var lineSeparatedError *action.Error
	if !errors.As(lineSeparated, &lineSeparatedError) || lineSeparatedError.RequestID != "" || lineSeparatedError.Scope != nil {
		t.Fatalf("line-separated request context escaped or retained prior values: %#v", lineSeparatedError)
	}
}

func TestPrepareDescriptorCanonicalizesAndOwnsErrorContract(t *testing.T) {
	descriptor := descriptorWithErrors(
		action.ErrorSpec{Code: "COUNTER.VERSION_CONFLICT", Kind: action.ErrorKindConflict},
		action.ErrorSpec{Code: "COUNTER.NOT_READY", Kind: action.ErrorKindPrecondition},
	)
	prepared, err := action.PrepareDescriptor(descriptor)
	if err != nil {
		t.Fatal(err)
	}

	descriptor.Errors[0] = action.ErrorSpec{Code: "FORGED.VALUE", Kind: action.ErrorKindInternal}
	first := prepared.Descriptor()
	if got, want := first.Errors, []action.ErrorSpec{
		{Code: "COUNTER.NOT_READY", Kind: action.ErrorKindPrecondition},
		{Code: "COUNTER.VERSION_CONFLICT", Kind: action.ErrorKindConflict},
	}; !equalErrorSpecs(got, want) {
		t.Fatalf("prepared errors = %#v, want %#v", got, want)
	}
	first.Errors[0] = action.ErrorSpec{Code: "MUTATED.COPY", Kind: action.ErrorKindInternal}
	if got := prepared.Descriptor().Errors[0].Code; got != "COUNTER.NOT_READY" {
		t.Fatalf("prepared descriptor mutated through returned error slice: %q", got)
	}

	data, err := json.Marshal(action.CatalogEntry{Descriptor: prepared.Descriptor(), ModuleID: "counter", ContractHash: prepared.ContractHash()})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"errors":[{"code":"COUNTER.NOT_READY","kind":"precondition"},{"code":"COUNTER.VERSION_CONFLICT","kind":"conflict"}]`) {
		t.Fatalf("catalog JSON does not expose the canonical error contract: %s", data)
	}
}

func TestDescriptorErrorContractValidationFailsClosed(t *testing.T) {
	validBoundaryCode := strings.Repeat("A", 31) + "." + strings.Repeat("B", 32)
	if err := action.ValidateDescriptor(descriptorWithErrors(action.ErrorSpec{Code: validBoundaryCode, Kind: action.ErrorKindConflict})); err != nil {
		t.Fatalf("64-byte boundary code rejected: %v", err)
	}

	tests := []struct {
		name   string
		errors []action.ErrorSpec
		match  string
	}{
		{name: "empty code", errors: []action.ErrorSpec{{Kind: action.ErrorKindConflict}}, match: "must match"},
		{name: "unqualified code", errors: []action.ErrorSpec{{Code: "OUT_OF_STOCK", Kind: action.ErrorKindConflict}}, match: "must match"},
		{name: "lowercase code", errors: []action.ErrorSpec{{Code: "inventory.OUT_OF_STOCK", Kind: action.ErrorKindConflict}}, match: "must match"},
		{name: "three segments", errors: []action.ErrorSpec{{Code: "INVENTORY.ITEM.OUT_OF_STOCK", Kind: action.ErrorKindConflict}}, match: "must match"},
		{name: "hyphen", errors: []action.ErrorSpec{{Code: "INVENTORY.OUT-OF-STOCK", Kind: action.ErrorKindConflict}}, match: "must match"},
		{name: "long namespace", errors: []action.ErrorSpec{{Code: strings.Repeat("A", 32) + ".B", Kind: action.ErrorKindConflict}}, match: "must match"},
		{name: "long name", errors: []action.ErrorSpec{{Code: "A." + strings.Repeat("B", 33), Kind: action.ErrorKindConflict}}, match: "must match"},
		{name: "unknown kind", errors: []action.ErrorSpec{{Code: "INVENTORY.OUT_OF_STOCK", Kind: action.ErrorKind("future")}}, match: "invalid kind"},
		{name: "zero kind", errors: []action.ErrorSpec{{Code: "INVENTORY.OUT_OF_STOCK"}}, match: "invalid kind"},
		{name: "internal kind", errors: []action.ErrorSpec{{Code: "INVENTORY.BROKEN", Kind: action.ErrorKindInternal}}, match: "internal kind"},
		{name: "duplicate", errors: []action.ErrorSpec{
			{Code: "INVENTORY.OUT_OF_STOCK", Kind: action.ErrorKindConflict},
			{Code: "INVENTORY.OUT_OF_STOCK", Kind: action.ErrorKindUnavailable},
		}, match: "declared more than once"},
		{name: "framework code", errors: []action.ErrorSpec{{Code: action.CodeValidationFailed, Kind: action.ErrorKindValidation}}, match: "owned by the framework"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := action.ValidateDescriptor(descriptorWithErrors(test.errors...))
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("ValidateDescriptor() error = %v, want containing %q", err, test.match)
			}
		})
	}

	tooMany := make([]action.ErrorSpec, 65)
	for index := range tooMany {
		tooMany[index] = action.ErrorSpec{Code: "BULK.ERROR_" + twoDigits(index), Kind: action.ErrorKindConflict}
	}
	if err := action.ValidateDescriptor(descriptorWithErrors(tooMany...)); err == nil || !strings.Contains(err.Error(), "more than 64") {
		t.Fatalf("oversized error contract error = %v", err)
	}
}

func TestContractHashBindsCanonicalErrorContract(t *testing.T) {
	first, err := action.PrepareDescriptor(descriptorWithErrors(
		action.ErrorSpec{Code: "COUNTER.VERSION_CONFLICT", Kind: action.ErrorKindConflict},
		action.ErrorSpec{Code: "COUNTER.NOT_READY", Kind: action.ErrorKindPrecondition},
	))
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := action.PrepareDescriptor(descriptorWithErrors(
		action.ErrorSpec{Code: "COUNTER.NOT_READY", Kind: action.ErrorKindPrecondition},
		action.ErrorSpec{Code: "COUNTER.VERSION_CONFLICT", Kind: action.ErrorKindConflict},
	))
	if err != nil {
		t.Fatal(err)
	}
	if first.ContractHash() != reordered.ContractHash() {
		t.Fatalf("equivalent error contracts hashed differently: %s != %s", first.ContractHash(), reordered.ContractHash())
	}

	changedKind, err := action.PrepareDescriptor(descriptorWithErrors(
		action.ErrorSpec{Code: "COUNTER.NOT_READY", Kind: action.ErrorKindUnavailable},
		action.ErrorSpec{Code: "COUNTER.VERSION_CONFLICT", Kind: action.ErrorKindConflict},
	))
	if err != nil {
		t.Fatal(err)
	}
	changedCode, err := action.PrepareDescriptor(descriptorWithErrors(
		action.ErrorSpec{Code: "COUNTER.NOT_READY", Kind: action.ErrorKindPrecondition},
		action.ErrorSpec{Code: "COUNTER.WRITE_CONFLICT", Kind: action.ErrorKindConflict},
	))
	if err != nil {
		t.Fatal(err)
	}
	if first.ContractHash() == changedKind.ContractHash() || first.ContractHash() == changedCode.ContractHash() {
		t.Fatal("error code or kind change did not change the Action contract hash")
	}

	withoutErrors, err := action.PrepareDescriptor(descriptorWithErrors())
	if err != nil {
		t.Fatal(err)
	}
	empty := descriptorWithErrors()
	empty.Errors = []action.ErrorSpec{}
	withEmptySlice, err := action.PrepareDescriptor(empty)
	if err != nil {
		t.Fatal(err)
	}
	if withoutErrors.ContractHash() != withEmptySlice.ContractHash() {
		t.Fatal("nil and empty error contracts hashed differently")
	}
}

func descriptorWithErrors(errors ...action.ErrorSpec) action.Descriptor {
	return action.Descriptor{
		ID:           "counter.increment",
		Version:      "1.0.0",
		Title:        "Increment counter",
		InputSchema:  action.Object(nil).JSON(),
		OutputSchema: action.Object(nil).JSON(),
		Permission:   "counter.increment",
		Preview:      action.PreviewNone,
		AuditLevel:   action.AuditMetadata,
		Channels:     []action.Channel{action.ChannelHTTP},
		Errors:       append([]action.ErrorSpec(nil), errors...),
	}
}

func equalErrorSpecs(first, second []action.ErrorSpec) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}

func twoDigits(value int) string {
	return string([]byte{'0' + byte(value/10), '0' + byte(value%10)})
}
