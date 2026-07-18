// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugin

const (
	// DefaultPriority is the default priority for transformation plugins.
	DefaultPriority = 10
)

// TransformBase provides common functionality for TransformPlugin implementations.
// Embed this type in your plugin struct to get default Meta() and Key() implementations.
//
// Example:
//
//	type LowerTransformPlugin struct {
//	    plugin.TransformBase
//	}
//
//	func NewLowerTransformPlugin() *LowerTransformPlugin {
//	    return &LowerTransformPlugin{
//	        TransformBase: plugin.NewTransformBase("lower", plugin.ForTypes("string")),
//	    }
//	}
//
//	func (t *LowerTransformPlugin) Apply(ctx plugin.GenerationContext, _ model.OptField, _ string, expr string) (string, bool) {
//	    ctx.AddImport("strings")
//	    return "strings.ToLower(" + expr + ")", true
//	}
type TransformBase struct {
	key             string
	applicableTypes []string
	priority        int
	defaultForTypes []string
	disabledBy      string
}

// TransformOption is a functional option for configuring a [TransformBase].
// See [ForTypes], [DefaultFor], and [DisabledBy].
type TransformOption func(*TransformBase)

// NewTransformBase creates a TransformBase with the given modifier key and
// functional options. The default priority is [DefaultPriority] (10).
func NewTransformBase(key string, opts ...TransformOption) TransformBase {
	b := TransformBase{
		key:      key,
		priority: DefaultPriority,
	}
	for _, opt := range opts {
		opt(&b)
	}
	return b
}

// ForTypes restricts the transform to the listed field types.
// When set, applying the transform to a non-matching type emits a warning
// and is silently skipped.
func ForTypes(types ...string) TransformOption {
	return func(b *TransformBase) {
		b.applicableTypes = types
	}
}

// DefaultFor marks types for which this transform is applied automatically,
// even without an explicit optval tag entry. Use [DisabledBy] to allow
// per-field opt-out.
func DefaultFor(types ...string) TransformOption {
	return func(b *TransformBase) {
		b.defaultForTypes = types
	}
}

// DisabledBy sets the optval key that suppresses automatic application of
// this transform. For example, DisabledBy("notrim") lets a field opt out of
// an automatic "trimspaces" transform by adding "notrim" to its optval tag.
func DisabledBy(key string) TransformOption {
	return func(b *TransformBase) {
		b.disabledBy = key
	}
}

// Meta returns the plugin metadata.
func (b *TransformBase) Meta() Meta {
	return Meta{
		Kind:            KindTransform,
		ApplicableTypes: b.applicableTypes,
		Priority:        b.priority,
		DefaultForTypes: b.defaultForTypes,
		DisabledBy:      b.disabledBy,
	}
}

// Key returns the modifier token used in optval tags.
func (b *TransformBase) Key() string {
	return b.key
}
