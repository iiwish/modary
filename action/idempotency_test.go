package action

import (
	"strings"
	"testing"
)

func TestValidateIdempotencyKeyUsesPortableChannelGrammar(t *testing.T) {
	for _, value := range []string{
		"a",
		"request-01.part_2~retry:scope/value+next",
		"x" + strings.Repeat("0", MaxIdempotencyKeyBytes-1),
	} {
		if err := ValidateIdempotencyKey(value); err != nil {
			t.Fatalf("ValidateIdempotencyKey(%q): %v", value, err)
		}
	}
	for _, value := range []string{
		"",
		" leading",
		"trailing ",
		"line\nbreak",
		"unicode-\u00e9",
		strings.Repeat("x", MaxIdempotencyKeyBytes+1),
	} {
		if err := ValidateIdempotencyKey(value); err == nil {
			t.Fatalf("ValidateIdempotencyKey(%q) succeeded", value)
		}
	}
}
