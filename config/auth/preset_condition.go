// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authconfig

import (
	"strings"

	"github.com/altessa-s/go-atlas/auth/oidc/rules"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// PresetCondition is a parsed preset-rule condition. Kind selects the matching operands.
type PresetCondition struct {
	Kind       string
	Claims     map[string]any
	Substrings map[string]string
	Names      []string
	Scope      string
	CEL        func(map[string]any) bool
	Children   []PresetCondition
}

// ParseConditions validates and parses this rule's matcher and its operands.
func (r *PresetRuleDefinition) ParseConditions() (PresetCondition, error) {
	return parseCondition(r.Conditions)
}

func parseCondition(conditions map[string]any) (PresetCondition, error) {
	if len(conditions) == 1 {
		for kind, value := range conditions {
			return parseOperand(kind, value)
		}
	}
	return PresetCondition{}, coreerrs.Wrap(rules.ErrInvalidConfig, "conditions must contain exactly one matcher")
}

func parseOperand(kind string, value any) (PresetCondition, error) {
	c := PresetCondition{Kind: kind}
	switch kind {
	case "claim_equals", "claim_contains":
		claims, ok := value.(map[string]any)
		if !ok || len(claims) == 0 {
			return c, coreerrs.Wrapf(rules.ErrInvalidConfig, "%s requires a non-empty object", kind)
		}
		if kind == "claim_equals" {
			c.Claims = claims
			break
		}
		c.Substrings = make(map[string]string, len(claims))
		for name, value := range claims {
			substring, ok := value.(string)
			if !ok {
				return c, coreerrs.Wrap(rules.ErrInvalidConfig, "claim_contains values must be strings")
			}
			c.Substrings[name] = substring
		}
	case "claim_exists", "has_any_scope", "has_all_scopes":
		var err error
		c.Names, err = parseNonEmptyStringArray(kind, value)
		if err != nil {
			return c, err
		}
	case "has_scope", "cel_expression":
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return c, coreerrs.Wrapf(rules.ErrInvalidConfig, "%s requires a non-empty string", kind)
		}

		if kind == "cel_expression" {
			var err error
			c.CEL, err = rules.CompileCEL(text)
			if err != nil {
				return c, coreerrs.Wrapf(rules.ErrInvalidCEL, "%v", err)
			}
		} else {
			c.Scope = text
		}
	case "matcher_and", "matcher_or":
		var err error
		c.Children, err = parseChildren(kind, value)
		if err != nil {
			return c, err
		}
	default:
		return c, coreerrs.Wrapf(rules.ErrInvalidConfig, "unknown condition type %q", kind)
	}
	return c, nil
}

func parseChildren(kind string, value any) ([]PresetCondition, error) {
	children, ok := value.([]any)
	if !ok || len(children) == 0 {
		return nil, coreerrs.Wrapf(rules.ErrInvalidConfig, "%s requires a non-empty array", kind)
	}
	parsed := make([]PresetCondition, 0, len(children))
	for i, item := range children {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, coreerrs.Wrapf(rules.ErrInvalidConfig, "%s[%d] must be an object", kind, i)
		}
		child, err := parseCondition(obj)
		if err != nil {
			return nil, coreerrs.Wrapf(err, "%s[%d]", kind, i)
		}
		parsed = append(parsed, child)
	}
	return parsed, nil
}

func parseNonEmptyStringArray(key string, value any) ([]string, error) {
	rawList, ok := value.([]any)
	if !ok || len(rawList) == 0 {
		return nil, coreerrs.Wrapf(rules.ErrInvalidConfig, "%s requires a non-empty array", key)
	}

	out := make([]string, len(rawList))
	for i, item := range rawList {
		s, ok := item.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, coreerrs.Wrapf(rules.ErrInvalidConfig, "%s array elements must be non-empty strings", key)
		}
		out[i] = s
	}
	return out, nil
}
