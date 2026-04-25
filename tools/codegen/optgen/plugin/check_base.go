// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugin

// CheckBase provides common functionality for CheckPlugin implementations.
// Embed this type in your plugin struct to get default Spec() and Aliases() implementations.
//
// Example:
//
//	type RequiredCheck struct {
//	    plugin.CheckBase
//	}
//
//	func NewRequiredCheck() *RequiredCheck {
//	    return &RequiredCheck{
//	        CheckBase: plugin.NewCheckBase("required", 10, plugin.WithAliases("nonempty")),
//	    }
//	}
//
//	func (c *RequiredCheck) Generate(ctx plugin.GenerationContext, field model.OptField, kind, valueVar, rawValue string) []string {
//	    // validation logic
//	}
type CheckBase struct {
	key           string
	priority      int
	requiresValue bool
	aliases       []string
}

// CheckOption is a functional option for configuring a [CheckBase] during
// construction. See [RequiresValue] and [WithAliases].
type CheckOption func(*CheckBase)

// NewCheckBase creates a new CheckBase with the given key, priority, and options.
func NewCheckBase(key string, priority int, opts ...CheckOption) CheckBase {
	b := CheckBase{
		key:      key,
		priority: priority,
	}
	for _, opt := range opts {
		opt(&b)
	}
	return b
}

// RequiresValue marks the check as requiring a parameter value in the tag
// (e.g. optcheck:"minlen=3"). Bare keys like "required" do not need this.
func RequiresValue() CheckOption {
	return func(b *CheckBase) {
		b.requiresValue = true
	}
}

// WithAliases registers alternative optcheck keys that resolve to the same
// check plugin (e.g. "nonempty" as an alias for "required").
func WithAliases(aliases ...string) CheckOption {
	return func(b *CheckBase) {
		b.aliases = aliases
	}
}

// Spec returns the check specification.
func (b *CheckBase) Spec() CheckSpec {
	return CheckSpec{
		Key:           b.key,
		Priority:      b.priority,
		RequiresValue: b.requiresValue,
	}
}

// Aliases returns alternative keys for this check.
func (b *CheckBase) Aliases() []string {
	return b.aliases
}
