// Package jsonschema owns the framework's offline Draft 7 admission graph,
// panic-contained compiler, and bounded flag-only validator. Callers decode one
// complete JSON document before preparing it.
package jsonschema

import (
	"errors"
	"fmt"

	engine "github.com/iiwish/modary/internal/jsonschema/engine"
)

// ErrEvaluationLimit identifies deterministic validator resource exhaustion.
var ErrEvaluationLimit = engine.ErrEvaluationLimit

// MaxEvaluationFrames bounds concurrently active recursive schema evaluations.
const MaxEvaluationFrames = engine.DefaultMaxEvaluationFrames

// Compiled is an immutable, concurrency-safe JSON Schema program.
type Compiled struct {
	schema *engine.Schema
}

// Compile profiles, freezes, and compiles an already parsed schema document.
func Compile(document any) (compiled *Compiled, err error) {
	return CompileWithLimits(document, DefaultCompileLimits())
}

// CompileWithLimits compiles with an explicit static schema profile. Runtime
// evaluation always retains the framework's fixed per-call budget.
func CompileWithLimits(document any, limits CompileLimits) (compiled *Compiled, err error) {
	completed := false
	defer func() {
		if completed {
			return
		}
		_ = recover()
		compiled = nil
		err = fmt.Errorf("JSON Schema compiler panicked")
	}()

	graph, err := PrepareWithLimits(document, limits)
	if err != nil {
		completed = true
		return nil, err
	}
	compiled, err = graph.Compile()
	if err != nil {
		completed = true
		return nil, err
	}
	completed = true
	return compiled, nil
}

// Compile compiles a prepared graph without rebuilding its policy profile.
func (graph *SchemaGraph) Compile() (compiled *Compiled, err error) {
	if graph == nil || graph.root == nil || !graph.metaschemaValid {
		return nil, fmt.Errorf("JSON Schema graph is not initialized")
	}
	completed := false
	defer func() {
		if completed {
			return
		}
		_ = recover()
		compiled = nil
		err = fmt.Errorf("JSON Schema compiler panicked")
	}()
	schema, err := engine.NewLocalSchema(graph.root, engine.Draft7)
	if err != nil {
		completed = true
		return nil, err
	}
	compiled = &Compiled{schema: schema}
	completed = true
	return compiled, nil
}

// ValidateFlag reports whether value satisfies the schema without constructing
// or formatting dependency diagnostics.
func (compiled *Compiled) ValidateFlag(value any) (valid bool, err error) {
	if compiled == nil || compiled.schema == nil {
		return false, fmt.Errorf("JSON Schema validator is not initialized")
	}
	completed := false
	defer func() {
		if completed {
			return
		}
		_ = recover()
		valid = false
		err = fmt.Errorf("JSON Schema validator panicked")
	}()

	valid, err = compiled.schema.ValidateFlag(value, engine.Budget{
		MaxWorkUnits:   MaxEvaluationWorkUnits,
		MaxDiagnostics: MaxGeneratedDiagnostics,
		MaxFrames:      MaxEvaluationFrames,
	})
	if err != nil {
		completed = true
		if errors.Is(err, engine.ErrEvaluationLimit) {
			return false, err
		}
		return false, fmt.Errorf("validate JSON Schema: %w", err)
	}
	completed = true
	return valid, nil
}
