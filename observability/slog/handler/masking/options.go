// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

// MaskFunc transforms a string value into its masked form.
// Built-in mask functions: [PartialMask], [SmartMask], [EmailMask],
// [FullMask], [FixedMask], [PatternMask], [HashMask], [PhoneMask],
// [CreditCardMask].
type MaskFunc func(value string) string

// FieldPattern configures regex-based matching for field names.
// Use [WithPattern] to add patterns to a [Handler].
type FieldPattern struct {
	Pattern string
	Mask    MaskFunc
}

// options configures masking behavior.
type options struct {
	// fields maps field names to their masking functions.
	fields map[string]MaskFunc `optgen:"manual"`

	// patterns contains regex patterns for matching field names.
	//
	// Note: patterns are matched against the field path when maskNestedFields is enabled,
	// otherwise against the field key only.
	patterns []FieldPattern `optgen:"manual"`

	// defaultMask is used when no specific mask is configured.
	defaultMask MaskFunc

	// maskNestedFields enables masking in nested groups.
	maskNestedFields bool

	// caseSensitive controls field name matching.
	caseSensitive bool
}

// WithField adds a field name to mask with the given MaskFunc.
func WithField(name string, mask MaskFunc) Option {
	return func(o *options) {
		if o.fields == nil {
			o.fields = make(map[string]MaskFunc)
		}
		o.fields[name] = mask
	}
}

// WithPattern adds a regex pattern for field name matching.
func WithPattern(pattern string, mask MaskFunc) Option {
	return func(o *options) {
		o.patterns = append(o.patterns, FieldPattern{Pattern: pattern, Mask: mask})
	}
}

// WithDefaults applies common sensitive field masks (password, token, secret, etc.).
// This is equivalent to the previous DefaultOptions() behavior.
func WithDefaults() Option {
	return func(o *options) {
		if o.fields == nil {
			o.fields = make(map[string]MaskFunc)
		}
		// Common sensitive fields
		o.fields["password"] = SmartMask()
		o.fields["token"] = SmartMask()
		o.fields["secret"] = SmartMask()
		o.fields["api_key"] = SmartMask()
		o.fields["authorization"] = SmartMask()
		o.fields["credit_card"] = CreditCardMask()
		o.fields["email"] = EmailMask()
		o.fields["phone"] = PhoneMask()

		// Common patterns
		o.patterns = append(o.patterns,
			FieldPattern{Pattern: `(?i).*password.*`, Mask: SmartMask()},
			FieldPattern{Pattern: `(?i).*secret.*`, Mask: SmartMask()},
			FieldPattern{Pattern: `(?i).*token.*`, Mask: SmartMask()},
			FieldPattern{Pattern: `(?i).*_key$`, Mask: SmartMask()},
		)

		o.defaultMask = SmartMask()
		o.maskNestedFields = true
		o.caseSensitive = true
	}
}
