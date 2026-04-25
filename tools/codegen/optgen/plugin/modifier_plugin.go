// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugin

import "github.com/altessa-s/go-atlas/tools/codegen/optgen/model"

// Phase represents the execution phase of a modifier in the pipeline.
// Modifiers are executed in phase order (lower phase runs first).
type Phase int

const (
	// PhaseGuard runs first. Guards can early-return from the option function.
	// Example: notnil - returns if input is nil.
	PhaseGuard Phase = 10

	// PhaseTransform modifies the input value.
	// Example: lower, upper, trimspaces - string transformations.
	PhaseTransform Phase = 20

	// PhasePostProcess runs after transforms for final cleanup.
	// Example: dedup - removes duplicates from slices.
	PhasePostProcess Phase = 30

	// PhaseCheck validates the final value before assignment.
	// Example: required, minlen, maxlen - validation checks.
	PhaseCheck Phase = 40
)

// String returns the phase name for display purposes.
func (p Phase) String() string {
	switch p {
	case PhaseGuard:
		return "guard"
	case PhaseTransform:
		return "transform"
	case PhasePostProcess:
		return "postprocess"
	case PhaseCheck:
		return "check"
	default:
		return "unknown"
	}
}

// ModifierResult is the output of [ModifierPlugin.Generate]. When Final is true
// the pipeline short-circuits and no subsequent phases run.
type ModifierResult struct {
	// Code is the generated code lines (without indentation).
	Code []string

	// OutputVar is the variable name after this modifier.
	// May be the same as inputVar if the modifier doesn't create a new variable.
	OutputVar string

	// Final indicates that pipeline should stop after this modifier.
	// Used by guards that early-return from the function.
	Final bool
}

// ModifierPlugin is the unified interface for all value modifiers.
// Modifiers are collected from optgen, optval, and optcheck tags
// and executed in phase order.
//
// Pipeline execution order:
//  1. PhaseGuard: Early return checks (e.g., notnil)
//  2. PhaseTransform: Value modifications (e.g., lower, upper, trimspaces)
//  3. PhasePostProcess: Cleanup operations (e.g., dedup, nonempty)
//  4. PhaseCheck: Validation (e.g., required, minlen, maxlen)
type ModifierPlugin interface {
	// Key returns the modifier name used in tags (e.g., "lower", "dedup", "required").
	Key() string

	// Phase returns when this modifier runs in the pipeline.
	Phase() Phase

	// CanHandle returns true if this modifier can handle the given field.
	// Use this to restrict modifiers to specific types (e.g., "lower" only for strings).
	CanHandle(field model.OptField) bool

	// Generate produces code for this modifier.
	// inputVar is the current value variable name.
	// Returns the generated code and the output variable name.
	Generate(ctx GenerationContext, field model.OptField, inputVar string) ModifierResult
}

// PipelineResult aggregates the output of a full modifier pipeline execution
// (see [Registry.BuildPipeline]).
type PipelineResult struct {
	// Code is the combined generated code from all modifiers.
	Code string

	// ValueVar is the final variable name to use for assignment.
	ValueVar string

	// EarlyReturn is true if a guard triggered an early return.
	EarlyReturn bool
}
