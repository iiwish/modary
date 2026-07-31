package action

import (
	"encoding/json"

	"github.com/iiwish/modary/internal/jsonvalue"
)

// Action JSON resource limits apply independently to every schema, request
// input, handler plan value, preview value, result, and persisted JSON value.
// Protocol envelopes have their own byte budget and revalidate embedded Action
// values against this contract.
const (
	MaxJSONDocumentBytes = int64(1 << 20)
	MaxJSONNestingDepth  = 256
	MaxJSONValueNodes    = 65_536
	MaxJSONNumberBytes   = 4_096
)

var actionJSONLimits = jsonvalue.Limits{
	MaxBytes:       MaxJSONDocumentBytes,
	MaxDepth:       MaxJSONNestingDepth,
	MaxNodes:       MaxJSONValueNodes,
	MaxNumberBytes: MaxJSONNumberBytes,
}

// ValidateJSONDocument checks one Action JSON document for the shared byte,
// nesting, node, numeric-token, UTF-8, single-value, and duplicate-member
// contract. Schema conformance is intentionally separate.
func ValidateJSONDocument(document json.RawMessage) error {
	return jsonvalue.Validate(document, actionJSONLimits)
}
