// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package modifiers

import (
	"reflect"
	"sync"
)

// Modifier defines a function that transforms a reflect.Value with optional parameters.
// If the modifier does not use parameters, it can ignore the params map.
//
// Example:
//
//	func MyModifier(v reflect.Value, params map[string]string) ModifierResult { ... }
type Modifier func(v reflect.Value, params map[string]string) ModifierResult

// ModifierResult represents the result of applying a [Modifier].
// It contains the modified value and an optional [ModifierError].
type ModifierResult struct {
	Value reflect.Value  // The modified value (zero value if modifier sets to nil)
	Error *ModifierError // Non-nil when the modifier encountered an error
}

// ModifierError represents an error that occurred during field modification.
// It includes context about the field, modifier, and original value.
// Use [errors.Is] to match the underlying Cause (e.g. [ErrPhoneParseFailure]).
type ModifierError struct {
	FieldName     string // Name of the struct field
	FieldPath     string // Full path to the field
	ModifierName  string // Name of the modifier that failed
	OriginalValue string // Original field value
	Cause         error  // Underlying error
}

// Error implements the error interface.
// Returns a formatted message including field path, modifier name, and cause.
func (e *ModifierError) Error() string {
	var message string
	if e.Cause != nil {
		message = e.Cause.Error()
	} else {
		message = "unknown error"
	}

	// Use optimized error message building
	return buildErrorMessage(e.FieldPath, e.FieldName, e.ModifierName, message)
}

// Unwrap returns the underlying error.
// Returns the Cause field for error chain traversal.
func (e *ModifierError) Unwrap() error {
	return e.Cause
}

// Is reports whether target matches the [ModifierError]'s Cause chain.
// This allows callers to use errors.Is(err, [ErrPhoneParseFailure]) directly
// against a wrapped [ModifierError].
func (e *ModifierError) Is(target error) bool {
	if target == nil {
		return false
	}

	// Check if the target matches our Cause
	if e.Cause != nil && e.Cause == target {
		return true
	}

	// Use the default Is behavior from the errors package
	if unwrapper, ok := e.Cause.(interface{ Unwrap() error }); ok && unwrapper != nil {
		return unwrapper.Unwrap() == target
	}

	return false
}

// buildErrorMessage creates an error message (simplified version)
func buildErrorMessage(fieldPath, fieldName, modifierName, message string) string {
	path := fieldPath
	if path == "" {
		path = fieldName
	}
	return "normalizer: error in field '" + path + "' (modifier '" + modifierName + "'): " + message
}

// StringTransformFunc is a function type that transforms a string.
// Returns the transformed string and true if the value was changed.
type StringTransformFunc func(string) (string, bool)

// ModifierParams represents parsed parameters from a modifier tag.
// It contains the modifier name and a map of parameter key-value pairs.
type ModifierParams struct {
	Name   string            // Modifier name
	Params map[string]string // Parameter key-value pairs
}

var (
	modifiersMu sync.RWMutex
	modifiers   = make(map[string]Modifier)
)

// RegisterModifier registers a named modifier function in the global registry.
// Safe for concurrent use. Overwrites any existing modifier with the same name.
//
// Example:
//
//	modifiers.RegisterModifier("custom", CustomModifier)
func RegisterModifier(name string, modifier Modifier) {
	modifiersMu.Lock()
	defer modifiersMu.Unlock()

	modifiers[name] = modifier
}

// GetModifier returns a modifier by name from the global registry.
// Returns the modifier and true if found, nil and false otherwise. Safe for concurrent use.
//
// Example:
//
//	mod, ok := modifiers.GetModifier("trim")
func GetModifier(name string) (Modifier, bool) {
	modifiersMu.RLock()
	defer modifiersMu.RUnlock()

	mod, ok := modifiers[name]
	return mod, ok
}

// NewModifierResult creates a new ModifierResult.
// Use this factory function to ensure consistent result creation.
//
// Example:
//
//	return NewModifierResult(reflect.ValueOf(result), nil)
func NewModifierResult(value reflect.Value, err *ModifierError) ModifierResult {
	return ModifierResult{
		Value: value,
		Error: err,
	}
}
