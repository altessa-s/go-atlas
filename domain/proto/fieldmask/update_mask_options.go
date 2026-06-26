// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

// PathValidationMode controls how [FieldMask.ApplyUpdateMask] treats field-mask
// paths that do not exist in the message schema (AIP-161). Set it via
// [WithPathValidation].
type PathValidationMode int

const (
	// PathValidationDisabled ignores schema-invalid paths, matching the historical
	// behavior: the path simply does not match any field and is skipped. This is
	// the default and keeps existing callers backward compatible.
	PathValidationDisabled PathValidationMode = iota

	// PathValidationWarn reports every schema-invalid path to the reporter set by
	// [WithPathValidationReporter] but still applies the mask, preserving the
	// Disabled behavior. Use it to surface bad paths (e.g. client typos) without
	// breaking callers while a fix rolls out. With no reporter it is a silent no-op.
	PathValidationWarn

	// PathValidationEnforce rejects the update with a *[ValidationError] before any
	// mutation, so a typo cannot clear a sibling subtree as a side effect (fail-fast).
	PathValidationEnforce
)

// ApplyOption is a functional option for configuring [FieldMask.ApplyUpdateMask].
type ApplyOption func(*applyOptions)

type applyOptions struct {
	pathValidation PathValidationMode
	pathReporter   func(*ValidationError)
}

func newApplyOptions(opts ...ApplyOption) applyOptions {
	var o applyOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// shouldValidatePaths reports whether ApplyUpdateMask must run the schema-path
// pass. Warn without a reporter has nowhere to surface results, so it is a
// no-op and skips the walk entirely.
func (o applyOptions) shouldValidatePaths() bool {
	switch o.pathValidation {
	case PathValidationEnforce:
		return true
	case PathValidationWarn:
		return o.pathReporter != nil
	default:
		return false
	}
}

// WithPathValidation sets how ApplyUpdateMask treats paths absent from the message
// schema (AIP-161). The default is [PathValidationDisabled].
func WithPathValidation(mode PathValidationMode) ApplyOption {
	return func(o *applyOptions) {
		o.pathValidation = mode
	}
}

// WithPathValidationReporter sets the sink invoked with the schema-invalid path when
// the mode is [PathValidationWarn]. It is ignored in the other modes.
func WithPathValidationReporter(report func(*ValidationError)) ApplyOption {
	return func(o *applyOptions) {
		o.pathReporter = report
	}
}
