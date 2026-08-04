package audit

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/modary/scope"
)

func TestInspectionPageEncodesDatabaseIDsWithoutJSONPrecisionLoss(t *testing.T) {
	encoded, err := json.Marshal(Page{
		Events: []Summary{{ID: math.MaxInt64}}, NextBeforeID: math.MaxInt64,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"id":"9223372036854775807"`, `"next_before_id":"9223372036854775807"`} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("inspection page JSON %s does not contain %s", encoded, field)
		}
	}
	empty, err := json.Marshal(Page{Events: []Summary{}})
	if err != nil || strings.Contains(string(empty), "next_before_id") {
		t.Fatalf("empty inspection page JSON = %s, %v", empty, err)
	}
}

func TestNormalizeBoundsAndSanitizesEnvelopeIdentifiers(t *testing.T) {
	event := Normalize(Event{
		RequestID:     strings.Repeat("r", MaxRequestIDRunes+10) + "\n",
		ActorID:       string([]byte{0xff}) + strings.Repeat("a", MaxActorIDRunes+10),
		ActorType:     "user\radmin",
		Channel:       "http\nheader",
		ActionID:      strings.Repeat("a", MaxActionIDRunes+10),
		ActionVersion: strings.Repeat("v", MaxVersionRunes+10) + "\r",
		ContractHash:  strings.Repeat("c", MaxHashRunes+10) + "\n",
		PlanHash:      strings.Repeat("h", MaxHashRunes+10),
		Scope:         scope.Execution{Kind: "tenant\n", ID: strings.Repeat("s", MaxScopeIDRunes+10)},
	})
	for field, value := range map[string]string{
		"request_id": event.RequestID, "actor_id": event.ActorID, "actor_type": event.ActorType,
		"channel": event.Channel, "action_id": event.ActionID, "action_version": event.ActionVersion,
		"contract_hash": event.ContractHash, "plan_hash": event.PlanHash,
		"scope.kind": event.Scope.Kind, "scope.id": event.Scope.ID,
	} {
		if !utf8.ValidString(value) || strings.ContainsFunc(value, unicode.IsControl) {
			t.Fatalf("%s was not sanitized: %q", field, value)
		}
	}
	if utf8.RuneCountInString(event.RequestID) > MaxRequestIDRunes || utf8.RuneCountInString(event.ActorID) > MaxActorIDRunes ||
		utf8.RuneCountInString(event.ActionID) > MaxActionIDRunes || utf8.RuneCountInString(event.ActionVersion) > MaxVersionRunes ||
		utf8.RuneCountInString(event.ContractHash) > MaxHashRunes || utf8.RuneCountInString(event.PlanHash) > MaxHashRunes ||
		utf8.RuneCountInString(event.Scope.ID) > MaxScopeIDRunes {
		t.Fatalf("normalized envelope remains unbounded: %#v", event)
	}
}

func TestNormalizeBoundsDetailedAuditData(t *testing.T) {
	resources := make([]string, MaxResources+5)
	references := make([]Reference, MaxReferences+5)
	for index := range resources {
		resources[index] = strings.Repeat("r", MaxResourceRunes+10)
		references[index] = Reference{Kind: "example.run", ID: strings.Repeat("i", MaxIDRunes+10)}
	}
	event := Normalize(Event{
		AuditLevel: "detailed", ResultSummary: strings.Repeat("界", MaxSummaryRunes+10),
		Reason: strings.Repeat("错", MaxReasonRunes+10),
		Impact: &Impact{Rows: -1, Resources: resources}, ResultRefs: references,
	})
	if utf8.RuneCountInString(event.ResultSummary) != MaxSummaryRunes || utf8.RuneCountInString(event.Reason) != MaxReasonRunes {
		t.Fatalf("unbounded event: summary=%d reason=%d", utf8.RuneCountInString(event.ResultSummary), utf8.RuneCountInString(event.Reason))
	}
	if event.Impact == nil || event.Impact.Rows != 0 || len(event.Impact.Resources) != MaxResources || len(event.ResultRefs) != MaxReferences {
		t.Fatalf("normalized detailed event = %#v", event)
	}
}

func TestNormalizeMetadataRemovesDetailedFields(t *testing.T) {
	event := Normalize(Event{
		AuditLevel: "metadata", Impact: &Impact{Rows: 10, Resources: []string{"table:secret"}},
		ResultSummary: "secret result summary",
		ResultRefs:    []Reference{{Kind: "example.run", ID: "run_1"}},
	})
	if event.ResultSummary != "" || event.Impact != nil || len(event.ResultRefs) != 0 {
		t.Fatalf("metadata event leaked detailed fields: %#v", event)
	}
}
