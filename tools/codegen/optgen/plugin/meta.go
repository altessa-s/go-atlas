// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugin

// Kind identifies the general category of a plugin. It is informational only
// (used by list-plugins output and debugging); dispatch is handled by type
// assertion in [Register].
type Kind string

const (
	KindField       Kind = "field"
	KindModifier    Kind = "modifier" // Unified modifier pipeline (guards, postprocess, etc.)
	KindTransform   Kind = "transform"
	KindOptCheck    Kind = "optcheck"
	KindTypeDefault Kind = "typedefault"
)

// Meta carries optional metadata that unifies behavior across all plugin types.
//
// All fields are zero-value safe. When a field is unset, the registry falls back
// to the plugin's own methods (e.g. CanHandle, Key). Embed a base type
// ([TransformBase], [CheckBase]) instead of implementing Meta manually.
type Meta struct {
	// Kind is informational only.
	Kind Kind

	// Name overrides reflection-based name used by list-plugins/disable-plugins/warnings.
	Name string

	// Priority controls ordering (higher runs first). Recommended priority ranges:
	//   100+  : User-explicit plugins (manual override, guard-style FieldPlugins)
	//   40-99 : Type-specific plugins (logger, net_ip_parse)
	//   20-39 : Modifier plugins (dedup, string transforms, bool_flag)
	//   0-19  : Fallback plugins (default setter, default appender)
	//   <0    : Passthrough/catch-all (-50)
	Priority int

	// ApplicableTypes limits the plugin to specific field types (as in model.OptField.Type).
	ApplicableTypes []string

	// Final indicates this modifier should short-circuit the pipeline (used by guard modifiers).
	Final bool

	// DefaultForTypes specifies types for which this transform plugin is applied automatically.
	// When set, the plugin's Key() is added to field modifiers even without explicit optval tag.
	// Only meaningful for TransformPlugin.
	// Example: []string{"string"} makes the plugin apply to all string fields by default.
	DefaultForTypes []string

	// DisabledBy is the modifier key that disables automatic (default) application.
	// When this modifier is present in optval, the plugin is not applied automatically.
	// Example: "notrim" disables automatic trimspaces for a field.
	DisabledBy string
}

// PluginMeta is an optional interface implemented by plugins to provide Meta().
type PluginMeta interface {
	Meta() Meta
}
