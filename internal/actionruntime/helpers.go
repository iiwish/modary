package actionruntime

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/internal/actionpersistence"
	"github.com/iiwish/modary/internal/jsonvalue"
	"github.com/iiwish/modary/internal/safeerr"
)

var actionJSONLimits = jsonvalue.Limits{
	MaxBytes:       action.MaxJSONDocumentBytes,
	MaxDepth:       action.MaxJSONNestingDepth,
	MaxNodes:       action.MaxJSONValueNodes,
	MaxNumberBytes: action.MaxJSONNumberBytes,
}

func cloneRequest(request action.Request) action.Request {
	request.Input = append(json.RawMessage(nil), request.Input...)
	return request
}

func cloneImpact(impact authz.Impact) authz.Impact {
	impact.Resources = append([]string(nil), impact.Resources...)
	return impact
}

func clonePlanData(data action.PlanData) action.PlanData {
	data.Payload = append(json.RawMessage(nil), data.Payload...)
	data.Summary = append(json.RawMessage(nil), data.Summary...)
	data.Impact = cloneImpact(data.Impact)
	return data
}

func clonePlan(plan action.Plan) action.Plan {
	plan.Payload = append(json.RawMessage(nil), plan.Payload...)
	plan.Impact = cloneImpact(plan.Impact)
	return plan
}

func cloneResult(result action.Result) action.Result {
	result.Data = append(json.RawMessage(nil), result.Data...)
	result.References = append([]audit.Reference(nil), result.References...)
	return result
}

func cloneIdempotencyRecord(record actionpersistence.IdempotencyRecord) actionpersistence.IdempotencyRecord {
	record.Impact = cloneImpact(record.Impact)
	record.Result = cloneResult(record.Result)
	return record
}

func decodeSingleJSON(data []byte) (any, error) {
	return jsonvalue.Decode(data, actionJSONLimits)
}

func findActionError(err error) (*action.Error, bool) {
	return safeerr.Find[*action.Error](err)
}

func safeErrorIs(err, target error) bool {
	return safeerr.Is(err, target)
}

func validateDescriptorText(field, value string, required bool, maxRunes int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", field)
	}
	if required && value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s cannot contain surrounding whitespace", field)
	}
	if utf8.RuneCountInString(value) > maxRunes {
		return fmt.Errorf("%s cannot exceed %d characters", field, maxRunes)
	}
	if strings.ContainsFunc(value, unicode.IsControl) {
		return fmt.Errorf("%s cannot contain control characters", field)
	}
	return nil
}
