// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

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

	// fieldOrder records every key [WithField] and [WithDefaults] set, in
	// registration order. [NewHandler] folds keys that differ only by case in
	// this order, so the last registration wins deterministically instead of
	// depending on map iteration order.
	fieldOrder []string `opt:"-"`

	// patterns contains regex patterns for matching field names.
	//
	// Note: patterns are matched against the field path when maskNestedFields is enabled,
	// otherwise against the field key only.
	patterns []FieldPattern `optgen:"manual"`

	// defaultMask is applied to rules registered without a mask, i.e.
	// [WithField] or [WithPattern] called with a nil [MaskFunc]. It does not
	// mask fields that no rule matches. [NewHandler] falls back to
	// [FullMask] when it is unset.
	defaultMask MaskFunc

	// maskNestedFields enables masking in nested groups.
	maskNestedFields bool

	// caseSensitive controls field name matching.
	caseSensitive bool
}

// WithField adds a field name to mask with the given MaskFunc. A nil mask
// applies the handler's default mask (see [WithDefaultMask]).
func WithField(name string, mask MaskFunc) Option {
	return func(o *options) {
		if o.fields == nil {
			o.fields = make(map[string]MaskFunc)
		}
		o.fields[name] = mask
		o.fieldOrder = append(o.fieldOrder, name)
	}
}

// WithPattern adds a regex pattern for field name matching. A nil mask
// applies the handler's default mask (see [WithDefaultMask]).
func WithPattern(pattern string, mask MaskFunc) Option {
	return func(o *options) {
		o.patterns = append(o.patterns, FieldPattern{Pattern: pattern, Mask: mask})
	}
}

// WithDefaults applies common sensitive field masks (password, token, secret, etc.)
// and enables nested-field masking.
//
// It does not change case sensitivity: field matching stays case-insensitive
// unless [WithCaseSensitive] is also given. It sets the default mask to
// [SmartMask] only when no [WithDefaultMask] was applied before it.
//
// Keys that differ only by case resolve to the last registration, so a
// [WithField] given after WithDefaults overrides the default for that field.
func WithDefaults() Option {
	return func(o *options) {
		if o.fields == nil {
			o.fields = make(map[string]MaskFunc)
		}
		// Common sensitive fields
		defaults := []struct {
			name string
			mask MaskFunc
		}{
			{"password", SmartMask()},
			{"token", SmartMask()},
			{"secret", SmartMask()},
			{"api_key", SmartMask()},
			{"authorization", SmartMask()},
			{"credit_card", CreditCardMask()},
			{"email", EmailMask()},
			{"phone", PhoneMask()},
		}
		for _, d := range defaults {
			o.fields[d.name] = d.mask
			o.fieldOrder = append(o.fieldOrder, d.name)
		}

		// Common patterns
		o.patterns = append(o.patterns,
			FieldPattern{Pattern: `(?i).*password.*`, Mask: SmartMask()},
			FieldPattern{Pattern: `(?i).*secret.*`, Mask: SmartMask()},
			FieldPattern{Pattern: `(?i).*token.*`, Mask: SmartMask()},
			FieldPattern{Pattern: `(?i).*_key$`, Mask: SmartMask()},
		)

		if o.defaultMask == nil {
			o.defaultMask = SmartMask()
		}
		o.maskNestedFields = true
	}
}
