// Package sqlpolicy provides a small, dialect-neutral SQL lexical boundary.
// It recognizes statement separators and unquoted ASCII words without trying
// to parse SQL grammar. Callers remain responsible for an explicit allowlist.
package sqlpolicy

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// ErrInvalidSQL reports input that cannot be safely classified as SQL text.
var ErrInvalidSQL = errors.New("SQL text is invalid")

// Token is one unquoted ASCII word, quoted identifier, quoted string,
// punctuation symbol, semicolon statement separator, or other executable SQL
// content. Whitespace and comments are skipped. String is retained only so a
// caller can reject dialects that permit quoted schema names; it remains Other.
type Token struct {
	Word       string
	Identifier string
	String     string
	Symbol     string
	Separator  bool
	Other      bool
}

// Tokenize returns the executable words and statement separators in text.
// maxBytes must be positive. NUL, invalid UTF-8, oversized input, and
// unterminated quoted values or block comments fail closed.
func Tokenize(text string, maxBytes int) ([]Token, error) {
	if maxBytes <= 0 || len(text) == 0 || len(text) > maxBytes || !utf8.ValidString(text) {
		return nil, ErrInvalidSQL
	}
	tokens := make([]Token, 0, 16)
	for index := 0; index < len(text); {
		switch current := text[index]; {
		case current == 0:
			return nil, ErrInvalidSQL
		case isSpace(current):
			index++
		case current == ';':
			tokens = append(tokens, Token{Separator: true})
			index++
		case current == '-' && index+1 < len(text) && text[index+1] == '-':
			index += 2
			for index < len(text) && text[index] != '\n' && text[index] != '\r' {
				if text[index] == 0 {
					return nil, ErrInvalidSQL
				}
				index++
			}
		case current == '/' && index+1 < len(text) && text[index+1] == '*':
			var ok bool
			index, ok = skipBlockComment(text, index+2)
			if !ok {
				return nil, ErrInvalidSQL
			}
		case current == '\'':
			start := index + 1
			var ok bool
			index, ok = skipDelimited(text, start, '\'')
			if !ok {
				return nil, ErrInvalidSQL
			}
			tokens = append(tokens, Token{
				String: decodeDelimitedIdentifier(text[start:index-1], '\''),
				Other:  true,
			})
		case current == '"':
			start := index + 1
			var ok bool
			index, ok = skipDelimited(text, start, '"')
			if !ok {
				return nil, ErrInvalidSQL
			}
			tokens = append(tokens, Token{
				Identifier: decodeDelimitedIdentifier(text[start:index-1], '"'),
				Other:      true,
			})
		case current == '`':
			start := index + 1
			var ok bool
			index, ok = skipDelimited(text, start, '`')
			if !ok {
				return nil, ErrInvalidSQL
			}
			tokens = append(tokens, Token{
				Identifier: decodeDelimitedIdentifier(text[start:index-1], '`'),
				Other:      true,
			})
		case current == '[':
			start := index + 1
			var ok bool
			index, ok = skipBracketIdentifier(text, start)
			if !ok {
				return nil, ErrInvalidSQL
			}
			tokens = append(tokens, Token{Identifier: text[start : index-1], Other: true})
		case isWordStart(current):
			start := index
			index++
			for index < len(text) && isWordContinue(text[index]) {
				index++
			}
			tokens = append(tokens, Token{Word: text[start:index]})
		default:
			_, size := utf8.DecodeRuneInString(text[index:])
			symbol := text[index : index+size]
			index += size
			tokens = append(tokens, Token{Symbol: symbol, Other: true})
		}
	}
	return tokens, nil
}

func decodeDelimitedIdentifier(value string, delimiter byte) string {
	pair := string([]byte{delimiter, delimiter})
	return strings.ReplaceAll(value, pair, string(delimiter))
}

func skipBlockComment(text string, index int) (int, bool) {
	for index < len(text) {
		if text[index] == 0 {
			return 0, false
		}
		if text[index] == '*' && index+1 < len(text) && text[index+1] == '/' {
			return index + 2, true
		}
		index++
	}
	return 0, false
}

func skipDelimited(text string, index int, delimiter byte) (int, bool) {
	for index < len(text) {
		if text[index] == 0 {
			return 0, false
		}
		if text[index] != delimiter {
			_, size := utf8.DecodeRuneInString(text[index:])
			index += size
			continue
		}
		if index+1 < len(text) && text[index+1] == delimiter {
			index += 2
			continue
		}
		return index + 1, true
	}
	return 0, false
}

func skipBracketIdentifier(text string, index int) (int, bool) {
	for index < len(text) {
		if text[index] == 0 {
			return 0, false
		}
		if text[index] == ']' {
			return index + 1, true
		}
		_, size := utf8.DecodeRuneInString(text[index:])
		index += size
	}
	return 0, false
}

func isSpace(value byte) bool {
	switch value {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	default:
		return false
	}
}

func isWordStart(value byte) bool {
	return value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func isWordContinue(value byte) bool {
	return isWordStart(value) || value >= '0' && value <= '9' || value == '$'
}
