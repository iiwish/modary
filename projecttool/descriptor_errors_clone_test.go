package projecttool

import (
	"testing"

	"github.com/iiwish/modary/action"
)

func TestInspectAndCatalogCopiesOwnDescriptorErrors(t *testing.T) {
	definition := fixtureDefinition(&inspectionCounters{}, false)
	declared := []action.ErrorSpec{
		{Code: "RECORDS.VERSION_CONFLICT", Kind: action.ErrorKindConflict},
		{Code: "RECORDS.NOT_READY", Kind: action.ErrorKindPrecondition},
	}
	definition.Modules[1].Definition.Actions[0].Descriptor.Errors = declared
	snapshot, err := Inspect(definition)
	if err != nil {
		t.Fatal(err)
	}
	entry := snapshotAction(t, snapshot, "records.create")
	if got := entry.Descriptor.Errors; len(got) != 2 || got[0].Code != "RECORDS.NOT_READY" || got[1].Code != "RECORDS.VERSION_CONFLICT" {
		t.Fatalf("canonical Descriptor errors = %#v", got)
	}

	declared[0] = action.ErrorSpec{Code: "FORGED.INPUT", Kind: action.ErrorKindUnavailable}
	if got := entry.Descriptor.Errors[1].Code; got != "RECORDS.VERSION_CONFLICT" {
		t.Fatalf("Snapshot was mutated through Definition input: %q", got)
	}

	copied := cloneCatalog(snapshot.Actions)
	snapshotAction(t, snapshot, "records.create").Descriptor.Errors[0] = action.ErrorSpec{
		Code: "FORGED.SOURCE", Kind: action.ErrorKindUnavailable,
	}
	if got := snapshotAction(t, Snapshot{Actions: copied}, "records.create").Descriptor.Errors[0].Code; got != "RECORDS.NOT_READY" {
		t.Fatalf("catalog copy was mutated through source: %q", got)
	}
	snapshotAction(t, Snapshot{Actions: copied}, "records.create").Descriptor.Errors[0] = action.ErrorSpec{
		Code: "FORGED.COPY", Kind: action.ErrorKindUnavailable,
	}
	if got := snapshotAction(t, snapshot, "records.create").Descriptor.Errors[0].Code; got != "FORGED.SOURCE" {
		t.Fatalf("catalog source was mutated through copy: %q", got)
	}
}

func snapshotAction(t *testing.T, snapshot Snapshot, id string) *action.CatalogEntry {
	t.Helper()
	for index := range snapshot.Actions {
		if snapshot.Actions[index].Descriptor.ID == id {
			return &snapshot.Actions[index]
		}
	}
	t.Fatalf("Snapshot does not contain Action %q", id)
	return nil
}
