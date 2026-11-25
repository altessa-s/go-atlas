// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"reflect"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/util"
)

type policySnapshot struct {
	preparedEval  rego.PreparedEvalQuery
	revision      string
	cacheEligible bool
}

// parseInput uses OPA's native input normalization once. The resulting AST
// belongs to this evaluation, including when the caller provides mutable maps.
func parseInput(input any) (ast.Value, error) {
	// EvalInput dereferences top-level pointers before its JSON conversion.
	for v := reflect.ValueOf(input); v.Kind() == reflect.Pointer; {
		v = v.Elem()
		if !v.IsValid() {
			return ast.NullValue, nil
		}
		input = v.Interface()
	}
	if err := util.RoundTripFast(&input); err != nil {
		return nil, err
	}
	return ast.InterfaceToValue(input)
}

// Check the compiled query and reachable rules, including every possible
// target of a dynamic document reference. An unanalyzable query runs uncached.
func cacheEligible(c *ast.Compiler, query string) bool {
	body, err := ast.ParseBody(query)
	if err != nil {
		return false
	}
	body, err = c.QueryCompiler().Compile(body)
	if err != nil {
		return false
	}
	seen := map[*ast.Rule]bool{}
	pending := []any{body}
	for len(pending) > 0 {
		last := len(pending) - 1
		node := pending[last]
		pending = pending[:last]
		deterministic := true
		ast.WalkRefs(node, func(ref ast.Ref) bool {
			if b := ast.BuiltinMap[ref.String()]; b != nil && b.Nondeterministic {
				deterministic = false
				return true
			}
			if ref.HasPrefix(ast.DefaultRootRef) {
				for _, rule := range c.GetRulesDynamicWithOpts(ref, ast.RulesOptions{IncludeHiddenModules: true}) {
					if !seen[rule] {
						seen[rule] = true
						pending = append(pending, rule)
					}
				}
			}
			return false
		})
		if !deterministic {
			return false
		}
	}
	return true
}
