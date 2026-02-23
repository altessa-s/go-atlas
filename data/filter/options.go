// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

// DefaultMaxDepth is the default maximum expression nesting depth.
const DefaultMaxDepth = 20

// TranslatorConfig holds configuration for translators.
type TranslatorConfig struct {
	allowedFields map[string]struct{}
	fieldMapping  map[string]string
	maxDepth      int
	strictMode    bool
}

// NewTranslatorConfig creates a TranslatorConfig with default values.
func NewTranslatorConfig() *TranslatorConfig {
	return &TranslatorConfig{
		maxDepth:   DefaultMaxDepth,
		strictMode: false,
	}
}

// TranslatorOption configures a translator.
type TranslatorOption func(*TranslatorConfig)

// WithAllowedFields sets a whitelist of allowed field names.
// When set, any field not in the list will cause translation to fail.
// This is a security feature to prevent unauthorized field access.
//
// Example:
//
//	trans := mongo.NewTranslator(filter.WithAllowedFields("name", "age", "email"))
func WithAllowedFields(fields ...string) TranslatorOption {
	return func(c *TranslatorConfig) {
		c.allowedFields = make(map[string]struct{}, len(fields))
		for _, f := range fields {
			c.allowedFields[f] = struct{}{}
		}
	}
}

// WithFieldMapping sets a mapping from CEL field names to database column names.
// This allows using user-friendly names in CEL while mapping to actual DB columns.
//
// Example:
//
//	trans := mongo.NewTranslator(filter.WithFieldMapping(map[string]string{
//	    "userName": "user_name",
//	    "createdAt": "created_at",
//	}))
func WithFieldMapping(mapping map[string]string) TranslatorOption {
	return func(c *TranslatorConfig) {
		c.fieldMapping = mapping
	}
}

// WithMaxDepth sets the maximum allowed expression nesting depth.
// This is a DoS protection to prevent deeply nested expressions from
// consuming excessive resources.
//
// Example:
//
//	trans := mongo.NewTranslator(filter.WithMaxDepth(10))
func WithMaxDepth(depth int) TranslatorOption {
	return func(c *TranslatorConfig) {
		if depth > 0 {
			c.maxDepth = depth
		}
	}
}

// WithStrictMode enables strict mode, which causes translation to fail
// on any unsupported operation rather than ignoring it.
//
// Example:
//
//	trans := mongo.NewTranslator(filter.WithStrictMode(true))
func WithStrictMode(strict bool) TranslatorOption {
	return func(c *TranslatorConfig) {
		c.strictMode = strict
	}
}

// ApplyFieldMapping applies the field mapping to a field name.
// Returns the mapped name if found, otherwise returns the original name.
func (c *TranslatorConfig) ApplyFieldMapping(field string) string {
	if c.fieldMapping == nil {
		return field
	}
	if mapped, ok := c.fieldMapping[field]; ok {
		return mapped
	}
	return field
}

// IsFieldAllowed checks if a field is allowed.
// Returns true if no allowlist is set or if the field is in the allowlist.
func (c *TranslatorConfig) IsFieldAllowed(field string) bool {
	if c.allowedFields == nil {
		return true
	}
	_, ok := c.allowedFields[field]
	return ok
}

// MaxDepth returns the configured maximum depth.
func (c *TranslatorConfig) MaxDepth() int {
	return c.maxDepth
}

// StrictMode returns whether strict mode is enabled.
func (c *TranslatorConfig) StrictMode() bool {
	return c.strictMode
}

// SetAllowedFields sets the allowed fields map.
func (c *TranslatorConfig) SetAllowedFields(fields map[string]struct{}) {
	c.allowedFields = fields
}

// SetFieldMapping sets the field mapping.
func (c *TranslatorConfig) SetFieldMapping(mapping map[string]string) {
	c.fieldMapping = mapping
}
