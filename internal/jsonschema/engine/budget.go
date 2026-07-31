package gojsonschema

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// ErrEvaluationLimit classifies deterministic validation resource exhaustion.
var ErrEvaluationLimit = errors.New("JSON Schema evaluation limit exceeded")

// DefaultMaxEvaluationFrames bounds active recursive validator calls when a
// caller leaves Budget.MaxFrames unset.
const DefaultMaxEvaluationFrames uint64 = 4_096

// Budget bounds one validation call. WorkUnits account for actual recursive
// visits, collection iterations, bytes scanned, regular-expression work, and
// numeric parsing. Diagnostics counts actual schema mismatches, including
// mismatches in discarded combinator branches. Frames bounds concurrently
// active recursive evaluations; zero selects DefaultMaxEvaluationFrames.
type Budget struct {
	MaxWorkUnits   uint64
	MaxDiagnostics uint64
	MaxFrames      uint64
}

// LimitError identifies the exhausted validation resource.
type LimitError struct {
	Resource string
	Limit    uint64
}

func (err *LimitError) Error() string {
	if err == nil {
		return ErrEvaluationLimit.Error()
	}
	return fmt.Sprintf("%s: %s exceeds %d", ErrEvaluationLimit, err.Resource, err.Limit)
}

func (err *LimitError) Unwrap() error {
	return ErrEvaluationLimit
}

type evaluationBudget struct {
	limits      Budget
	work        uint64
	diagnostics uint64
	frames      uint64
	exhausted   *LimitError
}

type limitAbort struct{}

func newEvaluationBudget(limits Budget) (*evaluationBudget, error) {
	if limits.MaxWorkUnits == 0 || limits.MaxDiagnostics == 0 {
		return nil, fmt.Errorf("JSON Schema evaluation limits must be positive")
	}
	if limits.MaxFrames == 0 {
		limits.MaxFrames = DefaultMaxEvaluationFrames
	}
	return &evaluationBudget{limits: limits}, nil
}

func (budget *evaluationBudget) consumeWork(units uint64) {
	if budget == nil || units == 0 {
		return
	}
	budget.work = saturatingAdd(budget.work, units)
	if budget.work > budget.limits.MaxWorkUnits {
		budget.exhausted = &LimitError{
			Resource: "work units",
			Limit:    budget.limits.MaxWorkUnits,
		}
		panic(limitAbort{})
	}
}

func (budget *evaluationBudget) consumeDiagnostic() {
	if budget == nil {
		return
	}
	budget.diagnostics = saturatingAdd(budget.diagnostics, 1)
	if budget.diagnostics > budget.limits.MaxDiagnostics {
		budget.exhausted = &LimitError{
			Resource: "generated diagnostics",
			Limit:    budget.limits.MaxDiagnostics,
		}
		panic(limitAbort{})
	}
}

func (budget *evaluationBudget) enterFrame() {
	if budget == nil {
		return
	}
	budget.frames = saturatingAdd(budget.frames, 1)
	if budget.frames > budget.limits.MaxFrames {
		budget.exhausted = &LimitError{
			Resource: "active frames",
			Limit:    budget.limits.MaxFrames,
		}
		panic(limitAbort{})
	}
}

func (budget *evaluationBudget) leaveFrame() {
	if budget == nil || budget.frames == 0 {
		return
	}
	budget.frames--
}

func (budget *evaluationBudget) limitError() error {
	if budget == nil || budget.exhausted == nil {
		return nil
	}
	return budget.exhausted
}

func saturatingAdd(left, right uint64) uint64 {
	if math.MaxUint64-left < right {
		return math.MaxUint64
	}
	return left + right
}

func saturatingMultiply(left, right uint64) uint64 {
	if left == 0 || right == 0 {
		return 0
	}
	if left > math.MaxUint64/right {
		return math.MaxUint64
	}
	return left * right
}

func numberWork(value json.Number) uint64 {
	digits := uint64(len(value))
	// big.Rat parsing and normalization are super-linear in the token size.
	work := saturatingAdd(1, saturatingMultiply(digits, digits))
	if index := strings.IndexAny(string(value), "eE"); index >= 0 {
		rawExponent := string(value)[index+1:]
		if strings.HasPrefix(rawExponent, "-") || strings.HasPrefix(rawExponent, "+") {
			rawExponent = rawExponent[1:]
		}
		exponent, err := strconv.ParseUint(rawExponent, 10, 64)
		if err != nil {
			return math.MaxUint64
		}
		work = saturatingAdd(work, saturatingMultiply(exponent, exponent))
	}
	return work
}

func rationalWork(value *big.Rat) uint64 {
	if value == nil {
		return 0
	}
	numeratorBytes := uint64((value.Num().BitLen() + 7) / 8)
	denominatorBytes := uint64((value.Denom().BitLen() + 7) / 8)
	operandBytes := saturatingAdd(numeratorBytes, denominatorBytes)
	return saturatingAdd(1, saturatingMultiply(operandBytes, operandBytes))
}

func regexpWork(expressionBytes, valueBytes uint64) uint64 {
	// Go's regexp engine is linear in the input for a fixed program. Charging
	// their product also covers the size of the compiled RE2 instruction set.
	return saturatingAdd(1, saturatingMultiply(
		saturatingAdd(1, expressionBytes),
		saturatingAdd(1, valueBytes),
	))
}
