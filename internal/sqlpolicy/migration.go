package sqlpolicy

import (
	"fmt"
	"strings"
)

var migrationStatementWords = map[string]struct{}{
	"ALTER":  {},
	"CREATE": {},
	"DELETE": {},
	"DROP":   {},
	"INSERT": {},
	"UPDATE": {},
}

// ValidateMigrationScript accepts a bounded sequence of durable DDL and DML
// statements. Transaction-control statements, temporary schema, REPLACE/WITH
// extensions, and executable ROLLBACK policy are outside this strict profile.
// CREATE TRIGGER bodies are recognized so their internal semicolons and
// BEGIN/END delimiters are not mistaken for top-level transaction control.
func ValidateMigrationScript(text string, maxBytes int) error {
	tokens, err := Tokenize(text, maxBytes)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		return ErrInvalidSQL
	}
	if HasTemporarySchemaReference(tokens) {
		return fmt.Errorf("%w: temporary schema is not durable", ErrInvalidSQL)
	}
	statements := 0
	for index := 0; index < len(tokens); {
		if tokens[index].Separator {
			return fmt.Errorf("%w: empty migration statement", ErrInvalidSQL)
		}
		if tokens[index].Word == "" {
			return fmt.Errorf("%w: migration statement must begin with a keyword", ErrInvalidSQL)
		}
		first := strings.ToUpper(tokens[index].Word)
		if _, allowed := migrationStatementWords[first]; !allowed {
			return fmt.Errorf("%w: migration statement %s is outside the durable DDL/DML profile", ErrInvalidSQL, first)
		}
		trigger, temporary := classifyCreate(tokens, index)
		if temporary {
			return fmt.Errorf("%w: temporary schema is not durable", ErrInvalidSQL)
		}
		if trigger {
			index, err = consumeTrigger(tokens, index)
		} else {
			index, err = consumeOrdinaryStatement(tokens, index)
		}
		if err != nil {
			return err
		}
		statements++
	}
	if statements == 0 {
		return ErrInvalidSQL
	}
	return nil
}

func classifyCreate(tokens []Token, start int) (trigger, temporary bool) {
	if !equalWord(tokens[start], "CREATE") {
		return false, false
	}
	words := make([]string, 0, 3)
	for index := start + 1; index < len(tokens) && len(words) < 3; index++ {
		if tokens[index].Separator {
			break
		}
		if tokens[index].Word != "" {
			words = append(words, strings.ToUpper(tokens[index].Word))
		}
	}
	if len(words) == 0 {
		return false, false
	}
	if words[0] == "TEMP" || words[0] == "TEMPORARY" {
		temporary = true
		if len(words) > 1 && words[1] == "TRIGGER" {
			trigger = true
		}
		return trigger, temporary
	}
	return words[0] == "TRIGGER", false
}

func consumeOrdinaryStatement(tokens []Token, start int) (int, error) {
	for index := start; index < len(tokens); index++ {
		if equalWord(tokens[index], "ROLLBACK") {
			return 0, fmt.Errorf("%w: ROLLBACK policy is not allowed in migrations", ErrInvalidSQL)
		}
		if !tokens[index].Separator {
			continue
		}
		if index+1 == len(tokens) {
			return len(tokens), nil
		}
		return index + 1, nil
	}
	return len(tokens), nil
}

func consumeTrigger(tokens []Token, start int) (int, error) {
	caseDepth := 0
	body := false
	atBodyStatementStart := false
	for index := start + 1; index < len(tokens); index++ {
		token := tokens[index]
		if equalWord(token, "ROLLBACK") {
			return 0, fmt.Errorf("%w: ROLLBACK policy is not allowed in migrations", ErrInvalidSQL)
		}
		if !body {
			if token.Separator {
				return 0, fmt.Errorf("%w: trigger has no BEGIN body", ErrInvalidSQL)
			}
			switch {
			case equalWord(token, "CASE"):
				caseDepth++
			case equalWord(token, "END") && caseDepth > 0:
				caseDepth--
			case equalWord(token, "BEGIN") && caseDepth == 0:
				body = true
				atBodyStatementStart = true
			}
			continue
		}

		if token.Separator {
			atBodyStatementStart = true
			continue
		}
		if equalWord(token, "CASE") {
			caseDepth++
			atBodyStatementStart = false
			continue
		}
		if equalWord(token, "END") {
			if caseDepth > 0 {
				caseDepth--
				atBodyStatementStart = false
				continue
			}
			if atBodyStatementStart {
				if index+1 == len(tokens) {
					return len(tokens), nil
				}
				if !tokens[index+1].Separator {
					return 0, fmt.Errorf("%w: unexpected SQL after trigger END", ErrInvalidSQL)
				}
				if index+2 == len(tokens) {
					return len(tokens), nil
				}
				return index + 2, nil
			}
		}
		atBodyStatementStart = false
	}
	return 0, fmt.Errorf("%w: unterminated trigger body", ErrInvalidSQL)
}

func equalWord(token Token, word string) bool {
	return token.Word != "" && strings.EqualFold(token.Word, word)
}
