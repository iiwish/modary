package identity

import (
	"reflect"
	"strings"
	"testing"
)

func TestActorKeepsAuthorizationPolicyOutOfIdentity(t *testing.T) {
	t.Parallel()

	typeOfActor := reflect.TypeOf(Actor{})
	want := []string{"ID", "Type", "DisplayName"}
	if typeOfActor.NumField() != len(want) {
		t.Fatalf("Actor has %d fields, want authentication identity only: %v", typeOfActor.NumField(), want)
	}
	for index, name := range want {
		if field := typeOfActor.Field(index); field.Name != name {
			t.Fatalf("Actor field %d = %s, want %s", index, field.Name, name)
		}
	}
}

func TestActorValidationDefinesTheCanonicalOpaqueEnvelope(t *testing.T) {
	valid := Actor{
		ID:          "01JABCDEF|user@example.test",
		Type:        "外部身份/service",
		DisplayName: "External User",
	}
	if err := ValidateActor(valid); err != nil {
		t.Fatalf("ValidateActor(valid) error = %v", err)
	}
	if err := ValidateActorID(strings.Repeat("界", MaxActorIDRunes)); err != nil {
		t.Fatalf("ValidateActorID(maximum) error = %v", err)
	}
	if err := ValidateActorType(strings.Repeat("界", MaxActorTypeRunes)); err != nil {
		t.Fatalf("ValidateActorType(maximum) error = %v", err)
	}
	if err := ValidateDisplayName(""); err != nil {
		t.Fatalf("empty optional display name error = %v", err)
	}

	tests := map[string]Actor{
		"missing id":      {Type: valid.Type},
		"oversized id":    {ID: strings.Repeat("界", MaxActorIDRunes+1), Type: valid.Type},
		"control type":    {ID: valid.ID, Type: "human\nadmin"},
		"spaced id":       {ID: " actor", Type: valid.Type},
		"invalid display": {ID: valid.ID, Type: valid.Type, DisplayName: "name\r"},
	}
	for name, actor := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ValidateActor(actor); err == nil {
				t.Fatalf("ValidateActor(%#v) succeeded", actor)
			}
		})
	}
}
