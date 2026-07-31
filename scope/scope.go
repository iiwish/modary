// Package scope defines validated consumer-owned execution identifiers used to
// isolate data, authorization, plans, idempotency records, and audit events.
// Modary assigns no domain meaning to the identifier values.
//
// Stability: alpha. Consumers should pin an exact pre-v1 Modary version.
package scope

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxKindRunes = 64
	maxIDRunes   = 256
)

var kindPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

// Execution identifies a consumer-defined isolation boundary. Kind names the
// boundary type, while ID is opaque to Modary.
type Execution struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// New validates and returns a consumer-defined execution scope.
func New(kind, id string) (Execution, error) {
	execution := Execution{Kind: kind, ID: id}
	if err := execution.Validate(); err != nil {
		return Execution{}, err
	}
	return execution, nil
}

// Must is New for programmer-owned literals and panics on an invalid value.
func Must(kind, id string) Execution {
	execution, err := New(kind, id)
	if err != nil {
		panic(err)
	}
	return execution
}

// Validate checks the portable kind grammar and bounded opaque identifier.
func (execution Execution) Validate() error {
	if !utf8.ValidString(execution.Kind) {
		return fmt.Errorf("execution scope kind must be valid UTF-8")
	}
	if !kindPattern.MatchString(execution.Kind) || utf8.RuneCountInString(execution.Kind) > maxKindRunes {
		return fmt.Errorf("execution scope kind must match %s", kindPattern.String())
	}
	if !utf8.ValidString(execution.ID) {
		return fmt.Errorf("execution scope id must be valid UTF-8")
	}
	if execution.ID == "" || strings.TrimSpace(execution.ID) != execution.ID || utf8.RuneCountInString(execution.ID) > maxIDRunes {
		return fmt.Errorf("execution scope id must contain 1 to %d non-surrounding-whitespace characters", maxIDRunes)
	}
	if strings.ContainsFunc(execution.ID, unicode.IsControl) {
		return fmt.Errorf("execution scope id cannot contain control characters")
	}
	return nil
}

// IsZero reports whether both scope fields are empty.
func (execution Execution) IsZero() bool {
	return execution.Kind == "" && execution.ID == ""
}

// String returns a human-readable kind/ID form and never serves as a composite
// persistence key.
func (execution Execution) String() string {
	if execution.IsZero() {
		return ""
	}
	return execution.Kind + "/" + execution.ID
}

// Key is collision-safe for persistence and in-memory composite keys.
func (execution Execution) Key() string {
	return execution.Kind + "\x00" + execution.ID
}
