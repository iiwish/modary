package scope

import (
	"encoding/json"
	"testing"
)

func TestExecutionValidationAndStableJSON(t *testing.T) {
	execution, err := New("tenant", "acme-west")
	if err != nil {
		t.Fatal(err)
	}
	if execution.Kind != "tenant" || execution.ID != "acme-west" {
		t.Fatalf("execution = %#v", execution)
	}
	data, err := json.Marshal(execution)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"kind":"tenant","id":"acme-west"}` {
		t.Fatalf("JSON = %s", data)
	}
	if execution.String() != "tenant/acme-west" {
		t.Fatalf("String = %q", execution.String())
	}
}

func TestExecutionRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		kind string
		id   string
	}{
		{name: "missing kind", id: "one"},
		{name: "missing id", kind: "tenant"},
		{name: "uppercase kind", kind: "Tenant", id: "one"},
		{name: "space in kind", kind: "work space", id: "one"},
		{name: "control in id", kind: "tenant", id: "one\nother"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.kind, test.id); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestExecutionRejectsInvalidUTF8(t *testing.T) {
	t.Parallel()

	for name, execution := range map[string]Execution{
		"kind": {Kind: string([]byte{0xff}), ID: "tenant-1"},
		"id":   {Kind: "tenant", ID: string([]byte{0xff})},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := execution.Validate(); err == nil {
				t.Fatal("expected invalid UTF-8 to be rejected")
			}
		})
	}
}

func TestMustPanicsForInvalidExecution(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Must did not panic")
		}
	}()
	_ = Must("", "missing-kind")
}
