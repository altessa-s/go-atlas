// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"maps"
	"sync"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// CustomFunction expands a CEL call of the form name(args...) into an
// arbitrary AST node. The handler receives already converted argument
// nodes and is responsible for validating arity and types. The node it
// returns replaces the original call in the AST before the filter
// reaches any [Evaluator] or translator.
//
// See [WithCustomFunctions] for registration and [CompareField] for the
// canonical "field op arg" shortcut.
type CustomFunction func(args []Node) (Node, error)

// CompareField returns a [CustomFunction] that expands name(arg) into
// BinaryOpNode{op, IdentNode(field), arg}. It is the canonical building
// block for semantic shortcuts such as createdAfter or updatedBefore:
//
//	filter.WithCustomFunctions(map[string]filter.CustomFunction{
//	    "createdAfter":  filter.CompareField("createdAt", filter.OpGT),
//	    "updatedBefore": filter.CompareField("updatedAt", filter.OpLT),
//	})
//
// op must be a comparison operator (see [Operator.IsComparison]);
// otherwise the handler returns an error wrapping
// [ErrInvalidExpression] when invoked.
func CompareField(field string, op Operator) CustomFunction {
	return func(args []Node) (Node, error) {
		if len(args) != 1 {
			return nil, coreerrs.Wrapf(ErrInvalidExpression,
				"function expects 1 argument, got %d", len(args))
		}
		if !op.IsComparison() {
			return nil, coreerrs.Wrapf(ErrInvalidExpression,
				"operator %v is not a comparison", op)
		}
		return &BinaryOpNode{Op: op, Left: &IdentNode{Name: field}, Right: args[0]}, nil
	}
}

// reservedFunctionNames lists CEL built-ins that callers must not
// override via [WithCustomFunctions]. Kept in sync with the explicit
// switch branches in [Parser.convertCall].
var reservedFunctionNames = coremaps.NewImmutableMap(map[string]struct{}{
	"contains":   {},
	"startsWith": {},
	"endsWith":   {},
	"matches":    {},
	"size":       {},
	"has":        {},
	"timestamp":  {},
})

// isUserFunction reports whether name is a user-callable named
// function subject to [WithAllowedFunctions]. CEL operator function
// ids start with _, @, or ! (e.g. _==_, @in, !_) and the has() macro
// is part of the baseline grammar — none of them are user functions.
func isUserFunction(name string) bool {
	if name == "" {
		return false
	}
	switch name[0] {
	case '_', '@', '!':
		return false
	}
	return name != "has"
}

// validateCustomFunctions rejects nil handlers and names that collide
// with reserved CEL built-ins. It is called from [NewParser] so a
// misconfiguration surfaces at construction time rather than during
// parsing.
func validateCustomFunctions(funcs map[string]CustomFunction) error {
	for name, h := range funcs {
		if h == nil {
			return coreerrs.Wrapf(ErrInvalidExpression,
				"custom function %q: nil handler", name)
		}
		if reservedFunctionNames.Contains(name) {
			return coreerrs.Wrapf(ErrInvalidExpression,
				"custom function %q collides with built-in", name)
		}
	}
	return nil
}

// globalCustomFunctions backs the package-level registry consulted by
// every [NewParser] call (unless the parser opts out via
// [WithoutGlobalCustomFunctions]). Reads are common; writes happen only
// during application bootstrap, so [sync.RWMutex] is the right tool.
var (
	globalCustomFunctionsMu sync.RWMutex
	globalCustomFunctions   = map[string]CustomFunction{}
)

// RegisterFunctions adds custom CEL functions to the package-level
// registry that every subsequent [NewParser] consults by default. It
// is intended to be called once during application bootstrap (main or
// init) so call sites need not repeat the registration in every
// [WithCustomFunctions].
//
// Validation matches [WithCustomFunctions]: nil handlers and names
// that collide with built-in CEL functions (contains, startsWith,
// endsWith, matches, size, has, timestamp) are rejected. Re-registering
// a name that is already in the registry returns an error — function
// values are not comparable in Go, so "same handler" cannot be
// distinguished from "different handler" idempotently. Use
// [ResetGlobalCustomFunctions] from tests that mutate the registry.
//
// Per-parser [WithCustomFunctions] still works and overrides any
// global registration with the same name. Use
// [WithoutGlobalCustomFunctions] to opt a single Parser out of the
// global set entirely.
func RegisterFunctions(funcs map[string]CustomFunction) error {
	if err := validateCustomFunctions(funcs); err != nil {
		return err
	}
	globalCustomFunctionsMu.Lock()
	defer globalCustomFunctionsMu.Unlock()
	for name := range funcs {
		if _, exists := globalCustomFunctions[name]; exists {
			return coreerrs.Wrapf(ErrInvalidExpression,
				"custom function %q already registered globally", name)
		}
	}
	maps.Copy(globalCustomFunctions, funcs)
	return nil
}

// GlobalCustomFunctions returns a snapshot of the package-level
// registry. Mutating the returned map has no effect on subsequently
// constructed parsers.
func GlobalCustomFunctions() map[string]CustomFunction {
	globalCustomFunctionsMu.RLock()
	defer globalCustomFunctionsMu.RUnlock()
	out := make(map[string]CustomFunction, len(globalCustomFunctions))
	maps.Copy(out, globalCustomFunctions)
	return out
}

// ResetGlobalCustomFunctions clears the package-level registry. It is
// intended for tests that register functions globally and need to
// restore baseline state — production code should not call it.
func ResetGlobalCustomFunctions() {
	globalCustomFunctionsMu.Lock()
	defer globalCustomFunctionsMu.Unlock()
	globalCustomFunctions = map[string]CustomFunction{}
}

// snapshotGlobalCustomFunctions returns a shallow copy of the registry
// for use during [NewParser] merge.
func snapshotGlobalCustomFunctions() map[string]CustomFunction {
	globalCustomFunctionsMu.RLock()
	defer globalCustomFunctionsMu.RUnlock()
	if len(globalCustomFunctions) == 0 {
		return nil
	}
	out := make(map[string]CustomFunction, len(globalCustomFunctions))
	maps.Copy(out, globalCustomFunctions)
	return out
}
