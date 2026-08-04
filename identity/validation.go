package identity

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// MaxActorIDRunes is the canonical bound for opaque principal identifiers.
	MaxActorIDRunes = 256
	// MaxActorTypeRunes is the canonical bound for consumer-defined actor types.
	MaxActorTypeRunes = 64
	// MaxDisplayNameRunes is the canonical bound for optional display names.
	MaxDisplayNameRunes = 256
	// MaxCredentialVersionRunes bounds an adapter-owned freshness value carried
	// only between authentication and session issuance.
	MaxCredentialVersionRunes = 256
)

// ValidateActor validates the complete identity envelope shared by Runtime and
// official identity and authorization adapters. Actor IDs and types are opaque
// trimmed UTF-8 text; Modary does not impose a provider-specific ASCII grammar.
func ValidateActor(actor Actor) error {
	if err := ValidateActorID(actor.ID); err != nil {
		return err
	}
	if err := ValidateActorType(actor.Type); err != nil {
		return err
	}
	if err := ValidateDisplayName(actor.DisplayName); err != nil {
		return err
	}
	return nil
}

// ValidateAuthentication validates an upstream authentication result without
// assigning authorization or product-scope meaning to it.
func ValidateAuthentication(authentication Authentication) error {
	if err := ValidateActor(authentication.Actor); err != nil {
		return err
	}
	switch authentication.Method {
	case AuthenticationMethodPassword:
		if err := validateActorText("credential version", authentication.CredentialVersion, true, MaxCredentialVersionRunes); err != nil {
			return err
		}
	case AuthenticationMethodOIDC:
		if authentication.CredentialVersion != "" {
			return fmt.Errorf("OIDC authentication cannot contain a password credential version")
		}
	default:
		return fmt.Errorf("authentication method is invalid")
	}
	return nil
}

// ValidateActorID validates an opaque actor identifier.
func ValidateActorID(value string) error {
	return validateActorText("actor id", value, true, MaxActorIDRunes)
}

// ValidateActorType validates a consumer-defined actor type.
func ValidateActorType(value string) error {
	return validateActorText("actor type", value, true, MaxActorTypeRunes)
}

// ValidateDisplayName validates an optional actor display name.
func ValidateDisplayName(value string) error {
	return validateActorText("actor display name", value, false, MaxDisplayNameRunes)
}

func validateActorText(name, value string, required bool, limit int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", name)
	}
	if required && value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s cannot contain surrounding whitespace", name)
	}
	if utf8.RuneCountInString(value) > limit {
		return fmt.Errorf("%s cannot exceed %d characters", name, limit)
	}
	if strings.ContainsFunc(value, unicode.IsControl) {
		return fmt.Errorf("%s cannot contain control characters", name)
	}
	return nil
}
