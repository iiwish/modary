package task_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/modary/task"
)

func TestInspectionPageEncodesDatabaseIDsWithoutJSONPrecisionLoss(t *testing.T) {
	encoded, err := json.Marshal(task.Page{
		Tasks: []task.Summary{{ID: math.MaxInt64}}, NextBeforeID: math.MaxInt64,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"id":"9223372036854775807"`, `"next_before_id":"9223372036854775807"`} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("inspection page JSON %s does not contain %s", encoded, field)
		}
	}
	empty, err := json.Marshal(task.Page{Tasks: []task.Summary{}})
	if err != nil || strings.Contains(string(empty), "next_before_id") {
		t.Fatalf("empty inspection page JSON = %s, %v", empty, err)
	}
}

func TestNormalizeRequestDefaultsAndCopies(t *testing.T) {
	payload := json.RawMessage(`{"run":"one"}`)
	request, err := task.NormalizeRequest(task.Request{Kind: "example.run", Payload: payload})
	if err != nil {
		t.Fatalf("NormalizeRequest() error = %v", err)
	}
	payload[2] = 'X'
	if got := string(request.Payload); got != `{"run":"one"}` {
		t.Fatalf("payload = %q", got)
	}
	if request.Queue != task.DefaultQueue || request.MaxAttempts != task.DefaultMaxAttempts {
		t.Fatalf("defaults = %#v", request)
	}
}

func TestNormalizeRequestRejectsInvalidInput(t *testing.T) {
	tests := []task.Request{
		{},
		{Kind: "Upper"},
		{Kind: "valid", Queue: "bad queue"},
		{Kind: "valid", Queue: "bad.queue"},
		{Kind: "valid", Queue: "bad__queue"},
		{Kind: "valid", Queue: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{Kind: "valid", MaxAttempts: -1},
		{Kind: "valid", Payload: json.RawMessage(`{`)},
		{Kind: "valid", UniqueKey: " padded "},
		{Kind: "valid", UniqueKey: "line\nbreak"},
	}
	for _, request := range tests {
		if _, err := task.NormalizeRequest(request); err == nil {
			t.Fatalf("NormalizeRequest(%#v) succeeded", request)
		}
	}
}

func TestNormalizeRunnerOptions(t *testing.T) {
	delays := []time.Duration{time.Second, 2 * time.Second}
	options, err := task.NormalizeRunnerOptions(task.RunnerOptions{RetryDelays: delays})
	if err != nil {
		t.Fatalf("NormalizeRunnerOptions() error = %v", err)
	}
	if len(options.Queues) != 1 || options.Queues[0].Name != task.DefaultQueue || options.Queues[0].MaxWorkers != task.DefaultMaxWorkers {
		t.Fatalf("queues = %#v", options.Queues)
	}
	delays[0] = time.Hour
	if options.RetryDelays[0] != time.Second {
		t.Fatal("retry delays alias caller storage")
	}
	if _, err := task.NormalizeRunnerOptions(task.RunnerOptions{Queues: []task.Queue{{Name: "same"}, {Name: "same"}}}); err == nil {
		t.Fatal("duplicate queues accepted")
	}
	if _, err := task.NormalizeRunnerOptions(task.RunnerOptions{Queues: []task.Queue{{Name: "not.river-compatible"}}}); err == nil {
		t.Fatal("River-incompatible queue name accepted")
	}
	if _, err := task.NormalizeRunnerOptions(task.RunnerOptions{RetryDelays: []time.Duration{-time.Second}}); err == nil {
		t.Fatal("negative retry delay accepted")
	}
	if _, err := task.NormalizeRunnerOptions(task.RunnerOptions{RetryDelays: []time.Duration{0}}); err == nil {
		t.Fatal("zero retry delay accepted")
	}
}

func TestNormalizeListOptionsUsesCanonicalStates(t *testing.T) {
	options, err := task.NormalizeListOptions(task.ListOptions{State: task.StateQueued})
	if err != nil || options.State != task.StateQueued {
		t.Fatalf("NormalizeListOptions(queued) = %#v, %v", options, err)
	}
	if _, err := task.NormalizeListOptions(task.ListOptions{State: task.State("available")}); err == nil {
		t.Fatal("NormalizeListOptions accepted provider-specific state")
	}
}

func TestJobTerminalAttempt(t *testing.T) {
	if (task.Job{Attempt: 2, MaxAttempts: 3}).TerminalAttempt() {
		t.Fatal("nonterminal attempt reported terminal")
	}
	if !(task.Job{Attempt: 3, MaxAttempts: 3}).TerminalAttempt() {
		t.Fatal("terminal attempt not reported")
	}
}
