// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcconfig

import middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"

// FieldMaskInterceptor defines the configuration for the update/read field
// mask interceptor. It applies the update_mask of Update requests to the
// resource before the handler runs and filters the responses of read methods
// by their read_mask. Per-method kinds and request extractors are code-level
// concerns and are supplied through the gRPC server factory.
type FieldMaskInterceptor struct {
	// BaseInterceptor provides common interceptor configuration.
	BaseInterceptor `yaml:",inline"`

	// SkipReadMask disables the response-side read_mask filtering of read
	// methods. Set it when the handlers already apply the read mask.
	SkipReadMask bool `yaml:"skipReadMask" default:"false"`

	// ApplyEmptyUpdateMask enables the empty-mask fallback: an update_mask that
	// is present but has no paths updates every populated field of the
	// resource instead of none.
	ApplyEmptyUpdateMask bool `yaml:"applyEmptyUpdateMask" default:"false"`

	// MaskFieldName overrides the conventional update_mask / read_mask request
	// field name. Empty uses update_mask / read_mask.
	MaskFieldName string `yaml:"maskFieldName"`

	// ResourceFieldName overrides the resource field of Update requests.
	// Empty picks the first non-mask message field declared on the request.
	ResourceFieldName string `yaml:"resourceFieldName"`

	// MetadataReadMaskHeader overrides the gRPC metadata key the read
	// mask is taken from. Empty uses "x-goog-fieldmask".
	MetadataReadMaskHeader string `yaml:"metadataReadMaskHeader"`
}

// IsEnabled returns true if the field mask interceptor is enabled.
func (c *FieldMaskInterceptor) IsEnabled() bool {
	return c != nil && c.Enabled
}

// Validate performs validation of the field mask interceptor configuration.
// Validates base configuration including method filtering.
//
// Returns an error if validation fails, nil otherwise.
func (c *FieldMaskInterceptor) Validate() error {
	return c.ValidateBase()
}

// DefaultFieldMaskInterceptor returns a FieldMaskInterceptor with default
// values. The interceptor is disabled by default.
func DefaultFieldMaskInterceptor() FieldMaskInterceptor {
	return FieldMaskInterceptor{
		BaseInterceptor: BaseInterceptor{
			EnableMixin: middlewareconfig.EnableMixin{Enabled: false},
		},
	}
}
