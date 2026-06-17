// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga

// Policy controls how strictly [Builder.Build] (and [New]) treat a
// compensatable step — one positioned before the pivot and not marked
// [ReadOnly] — that lacks a compensation. The check guards against silently
// shipping a saga whose rollback is incomplete.
type Policy int

const (
	// PolicyWarn (the default, zero value) lets Build succeed but causes [New]
	// to log a warning naming the steps that lack a compensation.
	PolicyWarn Policy = iota
	// PolicyEnforce makes [Builder.Build] return [errs.ErrNoCompensation] (and
	// MustBuild panic) when any compensatable step lacks a compensation.
	PolicyEnforce
	// PolicyDisabled turns the check off entirely.
	PolicyDisabled
)

// String returns a lowercase label for the policy.
func (p Policy) String() string {
	switch p {
	case PolicyWarn:
		return "warn"
	case PolicyEnforce:
		return "enforce"
	case PolicyDisabled:
		return "disabled"
	default:
		return "unknown"
	}
}

// buildConfig holds the resolved options for [Builder.Build].
type buildConfig struct {
	policy Policy
}

// BuildOption configures [Builder.Build].
type BuildOption func(*buildConfig)

// buildOptions applies opts on top of the defaults (PolicyWarn).
func buildOptions(opts ...BuildOption) buildConfig {
	cfg := buildConfig{policy: PolicyWarn}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WithCompensationPolicy sets the compensation-completeness policy for the
// definition. It is a build-time option because the check is a property of the
// definition's shape. The default is [PolicyWarn].
func WithCompensationPolicy(p Policy) BuildOption {
	return func(c *buildConfig) { c.policy = p }
}
