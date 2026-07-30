package rulary_core

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestExtractAddressLabelGoldenDataset(t *testing.T) {
	data, err := os.ReadFile("testdata/address_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name         string       `json:"name"`
		Input        string       `json:"input"`
		FilingMarker string       `json:"filing_marker"`
		Expected     AddressLabel `json:"expected"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			actual := ExtractAddressLabel(test.Input, test.FilingMarker)
			if !reflect.DeepEqual(actual, test.Expected) {
				t.Fatalf("label mismatch\nactual:   %+v\nexpected: %+v", actual, test.Expected)
			}
			runes := []rune(test.Input)
			for _, evidence := range actual.Evidence {
				if evidence.Start < 0 || evidence.End > len(runes) || evidence.Start >= evidence.End {
					t.Fatalf("invalid evidence offsets: %+v", evidence)
				}
				if got := string(runes[evidence.Start:evidence.End]); got != evidence.Text || !strings.Contains(test.Input, evidence.Text) {
					t.Fatalf("evidence is not source-backed: %+v, source slice %q", evidence, got)
				}
			}
		})
	}
}

func TestDefaultRuleSpecIsValid(t *testing.T) {
	if err := ValidateRuleSpec(DefaultRuleSpec()); err != nil {
		t.Fatal(err)
	}
}
