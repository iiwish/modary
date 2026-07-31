package action

import (
	"fmt"
	"regexp"
)

const (
	// MaxIdempotencyKeyBytes bounds the portable ASCII request token.
	MaxIdempotencyKeyBytes = 256
	// IdempotencyKeyPattern is the JSON Schema and Go validation grammar used
	// consistently by every Action channel.
	IdempotencyKeyPattern = `^[A-Za-z0-9][A-Za-z0-9._~:/+-]{0,255}$`
)

var idempotencyKeyPattern = regexp.MustCompile(IdempotencyKeyPattern)

// ValidateIdempotencyKey validates one non-empty portable Action retry token.
func ValidateIdempotencyKey(value string) error {
	if !idempotencyKeyPattern.MatchString(value) {
		return fmt.Errorf("idempotency key must match %s", IdempotencyKeyPattern)
	}
	return nil
}
