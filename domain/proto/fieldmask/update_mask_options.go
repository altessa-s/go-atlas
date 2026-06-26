// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

// ApplyOption is a functional option for configuring [FieldMask.ApplyUpdateMask].
type ApplyOption func(*applyOptions)

type applyOptions struct {
	validatePaths bool
}

func newApplyOptions(opts ...ApplyOption) applyOptions {
	var o applyOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithPathValidation makes ApplyUpdateMask reject field-mask paths absent from the
// message schema (AIP-161) instead of silently ignoring them. Off by default.
func WithPathValidation() ApplyOption {
	return func(o *applyOptions) {
		o.validatePaths = true
	}
}
