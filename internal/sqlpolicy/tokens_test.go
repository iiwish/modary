package sqlpolicy

import (
	"reflect"
	"testing"
)

func TestTokenizeSeparatesExecutableWordsFromQuotedAndCommentText(t *testing.T) {
	tokens, err := Tokenize("/* COMMIT */ UPDATE \"ROLL\"\"BACK\" SET value = 'END; BEGIN'; -- SAVEPOINT\n"+
		"DELETE FROM [COMMIT] JOIN `te``mp`.item", 4096)
	if err != nil {
		t.Fatal(err)
	}
	words := make([]string, 0)
	identifiers := make([]string, 0)
	separators := 0
	dots := 0
	for _, token := range tokens {
		if token.Word != "" {
			words = append(words, token.Word)
		}
		if token.Separator {
			separators++
		}
		if token.Identifier != "" {
			identifiers = append(identifiers, token.Identifier)
		}
		if token.Symbol == "." {
			dots++
		}
	}
	if want := []string{"UPDATE", "SET", "value", "DELETE", "FROM", "JOIN", "item"}; !reflect.DeepEqual(words, want) {
		t.Fatalf("words = %#v, want %#v", words, want)
	}
	if want := []string{`ROLL"BACK`, "COMMIT", "te`mp"}; !reflect.DeepEqual(identifiers, want) || separators != 1 || dots != 1 {
		t.Fatalf("identifiers = %#v, separators = %d, dots = %d", identifiers, separators, dots)
	}
}

func TestTokenizeRejectsMalformedOrUnboundedInput(t *testing.T) {
	for _, text := range []string{"", "SELECT\x00 1", string([]byte{0xff}), "/* missing", "'missing", `"missing`, "`missing", "[missing"} {
		if _, err := Tokenize(text, 1024); err == nil {
			t.Fatalf("Tokenize(%q) succeeded", text)
		}
	}
	if _, err := Tokenize("SELECT 1", 4); err == nil {
		t.Fatal("Tokenize accepted oversized text")
	}
}
