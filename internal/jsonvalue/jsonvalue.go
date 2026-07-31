// Package jsonvalue implements the shared, bounded JSON document boundary used
// by Action contracts and protocol envelopes.
package jsonvalue

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

// LimitError identifies which bounded JSON resource was exhausted.
type LimitError struct {
	Resource string
	Limit    int64
}

func (err *LimitError) Error() string {
	if err == nil {
		return "JSON resource limit exceeded"
	}
	return fmt.Sprintf("JSON document exceeds %d %s", err.Limit, err.Resource)
}

// IsLimit reports whether err contains a JSON resource limit failure.
func IsLimit(err error) bool {
	var target *LimitError
	return errors.As(err, &target)
}

// Limits bounds one complete JSON document. MaxDepth counts nested object and
// array containers, including the root container. MaxNodes counts every JSON
// value, including containers, but not object member names. MaxNumberBytes
// counts the source bytes in one JSON number token.
type Limits struct {
	MaxBytes       int64
	MaxDepth       int
	MaxNodes       int
	MaxNumberBytes int
}

// Validate checks that data is one bounded UTF-8 JSON value with no duplicate
// object member names.
func Validate(data []byte, limits Limits) error {
	_, err := Decode(data, limits)
	return err
}

// Decode validates data and returns maps, slices, primitives, and json.Number
// values without losing integer precision.
func Decode(data []byte, limits Limits) (any, error) {
	if err := validateLimits(limits); err != nil {
		return nil, err
	}
	if int64(len(data)) > limits.MaxBytes {
		return nil, &LimitError{Resource: "bytes", Limit: limits.MaxBytes}
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("JSON document is empty")
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("JSON document is not valid UTF-8")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	budget := decodeBudget{limits: limits}
	value, err := decodeValue(decoder, &budget, 0)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values are not allowed")
		}
		return nil, err
	}
	return value, nil
}

type decodeBudget struct {
	limits Limits
	nodes  int
}

func validateLimits(limits Limits) error {
	if limits.MaxBytes <= 0 || limits.MaxDepth <= 0 || limits.MaxNodes <= 0 || limits.MaxNumberBytes <= 0 {
		return fmt.Errorf("JSON byte, depth, node, and number limits must be positive")
	}
	return nil
}

func decodeValue(decoder *json.Decoder, budget *decodeBudget, depth int) (any, error) {
	budget.nodes++
	if budget.nodes > budget.limits.MaxNodes {
		return nil, &LimitError{Resource: "value nodes", Limit: int64(budget.limits.MaxNodes)}
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, nested := token.(json.Delim)
	if !nested {
		if number, ok := token.(json.Number); ok && len(number) > budget.limits.MaxNumberBytes {
			return nil, &LimitError{Resource: "number bytes", Limit: int64(budget.limits.MaxNumberBytes)}
		}
		return token, nil
	}
	if depth >= budget.limits.MaxDepth {
		return nil, &LimitError{Resource: "nested containers", Limit: int64(budget.limits.MaxDepth)}
	}

	switch delimiter {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			nameToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			name, ok := nameToken.(string)
			if !ok {
				return nil, fmt.Errorf("JSON object member name is not a string")
			}
			if _, duplicate := object[name]; duplicate {
				return nil, fmt.Errorf("duplicate JSON object property %q", name)
			}
			value, err := decodeValue(decoder, budget, depth+1)
			if err != nil {
				return nil, err
			}
			object[name] = value
		}
		if err := consumeClosingDelimiter(decoder, '}'); err != nil {
			return nil, err
		}
		return object, nil
	case '[':
		array := make([]any, 0)
		for decoder.More() {
			value, err := decodeValue(decoder, budget, depth+1)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if err := consumeClosingDelimiter(decoder, ']'); err != nil {
			return nil, err
		}
		return array, nil
	default:
		return nil, fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}

func consumeClosingDelimiter(decoder *json.Decoder, expected json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != expected {
		return fmt.Errorf("unexpected JSON closing delimiter %q", token)
	}
	return nil
}
