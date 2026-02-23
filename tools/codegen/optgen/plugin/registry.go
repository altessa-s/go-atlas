// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugin

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
)

// normalizeKey normalizes a plugin key to lowercase with trimmed spaces.
func normalizeKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// extractModifierKey extracts the key from a modifier string (before '=').
// Example: "positive=allow_zero" -> "positive"
func extractModifierKey(mod string) string {
	if idx := strings.IndexByte(mod, '='); idx > 0 {
		return mod[:idx]
	}
	return mod
}

// isPluginDisabledLocked checks if a plugin is disabled (caller must hold lock).
func (r *Registry) isPluginDisabledLocked(p any) bool {
	return r.disabledPlugins[normalizeKey(GetPluginName(p))]
}

// Registry is a concurrency-safe store for all plugin types. A process-wide
// default instance is used by the package-level functions ([Register],
// [FindPlugin], etc.). Create a private registry with [NewRegistry] only
// for testing or isolation.
type Registry struct {
	mu                 sync.RWMutex
	fieldPlugins       []FieldPlugin
	transformPlugins   map[string]TransformPlugin // key -> plugin (keys are normalized to lower)
	typeDefaultPlugins []TypeDefaultPlugin
	modifierPlugins    map[string]ModifierPlugin // key -> modifier plugin (unified pipeline)
	disabledPlugins    map[string]bool           // map of disabled plugin names
	checkSpecs         map[string]CheckSpec
	checkPlugins       map[string]CheckPlugin
}

// FieldPluginInfo is a display-oriented snapshot of a registered [FieldPlugin],
// used by the list-plugins command and debugging tools.
type FieldPluginInfo struct {
	Name     string
	Priority int
}

// NewRegistry creates a new plugin registry.
func NewRegistry() *Registry {
	return &Registry{
		fieldPlugins:       make([]FieldPlugin, 0),
		transformPlugins:   make(map[string]TransformPlugin),
		typeDefaultPlugins: make([]TypeDefaultPlugin, 0),
		modifierPlugins:    make(map[string]ModifierPlugin),
		disabledPlugins:    make(map[string]bool),
		checkSpecs:         make(map[string]CheckSpec),
		checkPlugins:       make(map[string]CheckPlugin),
	}
}

// RegisterTransformPlugins registers optval transform plugins in the registry.
// Keys are normalized to lower-case.
func (r *Registry) RegisterTransformPlugins(plugins ...TransformPlugin) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, p := range plugins {
		if p == nil {
			continue
		}
		if key := normalizeKey(p.Key()); key != "" {
			r.transformPlugins[key] = p
		}
	}
}

// FindTransformPlugin looks up a [TransformPlugin] by its normalized key and
// verifies that it is neither disabled nor type-restricted away from typeStr.
// The key may include a parameter suffix (e.g. "trimprefix=/"); the portion
// before '=' is used for lookup. Returns (nil, false) when no applicable
// transform is found.
func (r *Registry) FindTransformPlugin(key, typeStr string) (TransformPlugin, bool) {
	// Extract key from parameterized modifiers (e.g., "trimprefix=/" -> "trimprefix")
	k := normalizeKey(extractModifierKey(key))
	if k == "" {
		return nil, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.transformPlugins[k]
	if !ok || p == nil {
		return nil, false
	}
	if r.isPluginDisabledLocked(p) {
		return nil, false
	}
	if allowed := pluginApplicableTypes(p); len(allowed) > 0 && typeStr != "" && !typeAllowed(typeStr, allowed) {
		warnf("transform %s ignored for type %s: applicable types: %s", k, typeStr, strings.Join(allowed, ", "))
		return nil, false
	}
	return p, true
}

func appendAndSortTypeDefaults(dst *[]TypeDefaultPlugin, p TypeDefaultPlugin) {
	*dst = append(*dst, p)
	slices.SortFunc(*dst, func(a, b TypeDefaultPlugin) int {
		if c := cmp.Compare(b.Meta().Priority, a.Meta().Priority); c != 0 {
			return c
		}
		return cmp.Compare(GetPluginName(a), GetPluginName(b))
	})
}

// RegisterCheckPlugins registers optcheck plugins (and their specs) in the registry.
func (r *Registry) RegisterCheckPlugins(plugins ...CheckPlugin) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, h := range plugins {
		if h == nil {
			continue
		}
		spec := h.Spec()
		spec.Key = strings.TrimSpace(spec.Key)
		if spec.Key == "" {
			continue
		}
		r.checkSpecs[spec.Key] = spec
		r.checkPlugins[spec.Key] = h

		for _, a := range h.Aliases() {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			aliasSpec := spec
			aliasSpec.Key = a
			r.checkSpecs[a] = aliasSpec
			r.checkPlugins[a] = h
		}
	}
}

// CheckSpecs returns known optcheck specs (copy).
func (r *Registry) CheckSpecs() map[string]CheckSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.checkSpecs) == 0 {
		return nil
	}
	out := make(map[string]CheckSpec, len(r.checkSpecs))
	for k, v := range r.checkSpecs {
		out[k] = v
	}
	return out
}

// FindCheckPlugin finds a check plugin by key (or alias).
func (r *Registry) FindCheckPlugin(key string) (CheckPlugin, CheckSpec, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, CheckSpec{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.checkPlugins[key]
	if !ok {
		return nil, CheckSpec{}, false
	}
	spec, ok := r.checkSpecs[key]
	if !ok {
		return nil, CheckSpec{}, false
	}
	return h, spec, true
}

// KnownCheckKeys returns all registered optcheck keys (including aliases).
func (r *Registry) KnownCheckKeys() []string {
	m := r.CheckSpecs()
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func appendAndSortFieldPlugins(dst *[]FieldPlugin, p FieldPlugin) {
	*dst = append(*dst, p)
	slices.SortFunc(*dst, func(a, b FieldPlugin) int {
		if c := cmp.Compare(b.Meta().Priority, a.Meta().Priority); c != 0 {
			return c // higher priority first
		}
		return cmp.Compare(GetPluginName(a), GetPluginName(b))
	})
}

func pluginApplicableTypes(p any) []string {
	if pm, ok := p.(PluginMeta); ok {
		return pm.Meta().ApplicableTypes
	}
	return nil
}

func sortedPluginNames[T any](items []T) []string {
	out := make([]string, 0, len(items))
	for _, p := range items {
		out = append(out, GetPluginName(p))
	}
	slices.Sort(out)
	return out
}

// RegisterFieldPlugin registers a field generator plugin.
// Plugins are sorted by priority (highest first) after registration.
func (r *Registry) RegisterFieldPlugin(p FieldPlugin) {
	r.mu.Lock()
	defer r.mu.Unlock()
	appendAndSortFieldPlugins(&r.fieldPlugins, p)
}

// DisablePlugins disables plugins by name.
// Disabled plugins will be skipped during plugin search and post-processing.
func (r *Registry) DisablePlugins(names ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, name := range names {
		if key := normalizeKey(name); key != "" {
			r.disabledPlugins[key] = true
		}
	}
}

// IsPluginDisabled checks if a plugin is disabled.
func (r *Registry) IsPluginDisabled(p any) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.isPluginDisabledLocked(p)
}

// FindPlugin finds the best plugin for a field.
// Returns the first plugin that can handle the field (sorted by priority).
// Skips disabled plugins.
//
// Returns an error if no plugin can handle the field.
func (r *Registry) FindPlugin(field model.OptField) (FieldPlugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.fieldPlugins {
		if r.isPluginDisabledLocked(p) {
			continue
		}
		match := p.CanHandle(field)

		// If the plugin declares applicable types and this field type is not allowed,
		// ignore it. If it otherwise matches (CanHandle returns true), emit a warning.
		if allowed := pluginApplicableTypes(p); len(allowed) > 0 && !typeAllowed(field.Type, allowed) {
			if match {
				warnf("field plugin %s ignored for field %s (type %s): applicable types: %s",
					GetPluginName(p), field.FieldName, field.Type, strings.Join(allowed, ", "))
			}
			continue
		}
		if match {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no plugin found for field %s (type: %s)", field.FieldName, field.Type)
}

func typeAllowed(typeStr string, allowed []string) bool {
	for _, t := range allowed {
		t = strings.TrimSpace(t)
		// Special pattern: "[]" matches any slice type
		if t == "[]" && strings.HasPrefix(typeStr, "[]") {
			return true
		}
		// Special pattern: "*" matches any pointer type
		if t == "*" && strings.HasPrefix(typeStr, "*") {
			return true
		}
		// Special pattern: "interface" matches interface{} or any type
		if t == "interface" && (typeStr == "interface{}" || typeStr == "any") {
			return true
		}
		if t == typeStr {
			return true
		}
	}
	return false
}

// Global registry for convenient access.
var defaultRegistry = NewRegistry()

// Register registers one or more plugins in the global registry.
// The plugin type is determined automatically via type assertion.
// This is the recommended function for use in init() functions.
//
// Supported plugin types:
//   - ModifierPlugin (guards, postprocessors via unified pipeline)
//   - FieldPlugin
//   - TransformPlugin
//   - TypeDefaultPlugin
//   - CheckPlugin
//
// Panics if an unsupported plugin type is passed.
func Register(plugins ...any) {
	for _, p := range plugins {
		switch v := p.(type) {
		case ModifierPlugin:
			defaultRegistry.RegisterModifierPlugin(v)
		case FieldPlugin:
			defaultRegistry.RegisterFieldPlugin(v)
		case TransformPlugin:
			defaultRegistry.RegisterTransformPlugins(v)
		case TypeDefaultPlugin:
			defaultRegistry.RegisterTypeDefaultPlugin(v)
		case CheckPlugin:
			defaultRegistry.RegisterCheckPlugins(v)
		default:
			panic(fmt.Sprintf("plugin.Register: unsupported plugin type %T", p))
		}
	}
}

// FindPlugin finds a plugin for a field in the global registry.
func FindPlugin(field model.OptField) (FieldPlugin, error) {
	return defaultRegistry.FindPlugin(field)
}

// DisablePlugins disables plugins by name in the global registry.
func DisablePlugins(names ...string) {
	defaultRegistry.DisablePlugins(names...)
}

// RegisterTypeDefaultPlugin registers a type default plugin in the registry.
// Plugins are sorted by priority (highest first) after registration.
func (r *Registry) RegisterTypeDefaultPlugin(p TypeDefaultPlugin) {
	r.mu.Lock()
	defer r.mu.Unlock()
	appendAndSortTypeDefaults(&r.typeDefaultPlugins, p)
}

// FindTypeDefaultPlugin finds a default value plugin for a type.
// Returns the first plugin that can handle the type (sorted by priority).
// Skips disabled plugins.
//
// Returns empty string if no plugin can handle the type.
func (r *Registry) FindTypeDefaultPlugin(typeStr string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.typeDefaultPlugins {
		if r.isPluginDisabledLocked(p) {
			continue
		}
		if p.CanProvideDefault(typeStr) {
			return p.GetDefault(typeStr)
		}
	}
	return ""
}

// KnownPluginNames returns the type names of all registered plugins.
// Names are sorted and intended for CLI validation/help.
func (r *Registry) KnownPluginNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]bool)
	for _, p := range r.fieldPlugins {
		seen[GetPluginName(p)] = true
	}
	for _, p := range r.modifierPlugins {
		seen[GetPluginName(p)] = true
	}
	for _, p := range r.transformPlugins {
		seen[GetPluginName(p)] = true
	}
	for _, p := range r.typeDefaultPlugins {
		seen[GetPluginName(p)] = true
	}

	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// KnownPluginNames returns type names of all plugins/providers in the global registry.
func KnownPluginNames() []string {
	return defaultRegistry.KnownPluginNames()
}

// KnownFieldPluginNames returns the type names of registered field plugins.
func (r *Registry) KnownFieldPluginNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sortedPluginNames(r.fieldPlugins)
}

// FieldPlugins returns registered field plugins with metadata, in selection order
// (guards first, then descending priority).
func (r *Registry) FieldPlugins() []FieldPluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.fieldPlugins) == 0 {
		return nil
	}
	out := make([]FieldPluginInfo, 0, len(r.fieldPlugins))
	for _, p := range r.fieldPlugins {
		out = append(out, FieldPluginInfo{
			Name:     GetPluginName(p),
			Priority: p.Meta().Priority,
		})
	}
	return out
}

// KnownFieldPluginNames returns the type names of field plugins in the global registry.
func KnownFieldPluginNames() []string {
	return defaultRegistry.KnownFieldPluginNames()
}

// FieldPlugins returns registered field plugins with metadata from the global registry.
func FieldPlugins() []FieldPluginInfo {
	return defaultRegistry.FieldPlugins()
}

// KnownTypeDefaultPluginNames returns the type names of registered type default plugins.
func (r *Registry) KnownTypeDefaultPluginNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sortedPluginNames(r.typeDefaultPlugins)
}

// KnownTypeDefaultPluginNames returns the plugin names in the global registry.
func KnownTypeDefaultPluginNames() []string {
	return defaultRegistry.KnownTypeDefaultPluginNames()
}

// FindTypeDefaultPlugin finds a default value for a type in the global registry.
func FindTypeDefaultPlugin(typeStr string) string {
	return defaultRegistry.FindTypeDefaultPlugin(typeStr)
}

// RegisterModifierPlugin registers a modifier plugin in the registry.
// Keys are normalized to lower-case.
func (r *Registry) RegisterModifierPlugin(p ModifierPlugin) {
	if p == nil {
		return
	}
	key := normalizeKey(p.Key())
	if key == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modifierPlugins[key] = p
}

// FindModifierPlugin finds a modifier plugin by key.
// Returns nil if no plugin is registered for the key.
func (r *Registry) FindModifierPlugin(key string) ModifierPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.modifierPlugins[normalizeKey(key)]
}

// CollectModifiers gathers all [ModifierPlugin] instances that apply to field,
// sourced from three tag namespaces: optgen metadata (guards like "notnil"),
// optval modifiers (transforms and post-processors), and optcheck entries.
// The result is stable-sorted by [Phase]; within the same phase, tag declaration
// order is preserved. Disabled plugins and plugins whose CanHandle returns false
// are excluded.
func (r *Registry) CollectModifiers(field model.OptField) []ModifierPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Pre-allocate with estimated capacity
	capacity := len(field.Metadata) + len(field.Modifiers) + len(field.Checks)
	modifiers := make([]ModifierPlugin, 0, capacity)

	// tryAdd attempts to add a modifier plugin by key
	tryAdd := func(key string) {
		if p := r.modifierPlugins[normalizeKey(key)]; p != nil {
			if p.CanHandle(field) && !r.isPluginDisabledLocked(p) {
				modifiers = append(modifiers, p)
			}
		}
	}

	// Collect from optgen metadata (guards like "notnil")
	for key := range field.Metadata {
		tryAdd(key)
	}

	// Collect from optval modifiers (transforms, postprocess)
	for _, mod := range field.Modifiers {
		tryAdd(extractModifierKey(mod))
	}

	// Collect from optcheck (validation checks)
	for _, check := range field.Checks {
		tryAdd(extractModifierKey(check))
	}

	// Sort by phase (stable sort preserves tag order within each phase)
	slices.SortStableFunc(modifiers, func(a, b ModifierPlugin) int {
		return cmp.Compare(a.Phase(), b.Phase())
	})

	return modifiers
}

// BuildPipeline executes the modifier pipeline for a field.
// Pipeline phases:
//  1. PhaseGuard: Early return checks (ModifierPlugins)
//  2. PhaseTransform: String transforms (uses TransformPlugins internally)
//  3. PhasePostProcess: Cleanup operations (ModifierPlugins)
//  4. PhaseCheck: Validation checks (ModifierPlugins)
func (r *Registry) BuildPipeline(ctx GenerationContext, field model.OptField, inputVar string) PipelineResult {
	modifiers := r.CollectModifiers(field)
	if len(modifiers) == 0 {
		return PipelineResult{ValueVar: inputVar}
	}

	currentVar := inputVar
	codeLines := make([]string, 0, len(modifiers)*3) // estimate 3 lines per modifier

	// processPhase runs modifiers for a given phase
	processPhase := func(phase Phase) bool {
		for _, mod := range modifiers {
			if mod.Phase() != phase {
				continue
			}
			result := mod.Generate(ctx, field, currentVar)
			if len(result.Code) > 0 {
				codeLines = append(codeLines, result.Code...)
			}
			if result.OutputVar != "" {
				currentVar = result.OutputVar
			}
			if result.Final {
				return true // early return
			}
		}
		return false
	}

	// Phase 1: Guards
	if processPhase(PhaseGuard) {
		return PipelineResult{
			Code:        strings.Join(codeLines, "\n"),
			ValueVar:    currentVar,
			EarlyReturn: true,
		}
	}

	// Phase 2: Transforms - handled by caller (optionData.BuildTransform)

	// Phase 3: PostProcess
	processPhase(PhasePostProcess)

	// Phase 4: Checks
	processPhase(PhaseCheck)

	var code string
	if len(codeLines) > 0 {
		code = strings.Join(codeLines, "\n")
	}

	return PipelineResult{
		Code:     code,
		ValueVar: currentVar,
	}
}

// BuildPipeline executes the modifier pipeline in the global registry.
func BuildPipeline(ctx GenerationContext, field model.OptField, inputVar string) PipelineResult {
	return defaultRegistry.BuildPipeline(ctx, field, inputVar)
}

// CollectModifiers collects all applicable modifier plugins from the global registry.
func CollectModifiers(field model.OptField) []ModifierPlugin {
	return defaultRegistry.CollectModifiers(field)
}

// FindModifierPlugin finds a modifier plugin by key in the global registry.
func FindModifierPlugin(key string) ModifierPlugin {
	return defaultRegistry.FindModifierPlugin(key)
}

// ModifierPluginInfo is a display-oriented snapshot of a registered [ModifierPlugin],
// used by the list-plugins command and debugging tools.
type ModifierPluginInfo struct {
	Name  string
	Key   string
	Phase Phase
}

// ModifierPlugins returns registered modifier plugins with metadata.
func (r *Registry) ModifierPlugins() []ModifierPluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.modifierPlugins) == 0 {
		return nil
	}
	out := make([]ModifierPluginInfo, 0, len(r.modifierPlugins))
	for key, p := range r.modifierPlugins {
		out = append(out, ModifierPluginInfo{
			Name:  GetPluginName(p),
			Key:   key,
			Phase: p.Phase(),
		})
	}
	slices.SortFunc(out, func(a, b ModifierPluginInfo) int {
		if c := cmp.Compare(a.Phase, b.Phase); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return out
}

// ModifierPlugins returns registered modifier plugins with metadata from the global registry.
func ModifierPlugins() []ModifierPluginInfo {
	return defaultRegistry.ModifierPlugins()
}

// GetDefaultModifiersForType returns modifier keys that should be applied
// automatically for typeStr, based on plugins whose [Meta].DefaultForTypes
// matches. Modifiers already present in existingMods are excluded, as are those
// whose DisabledBy key appears in existingMods. The result is sorted by
// descending priority.
func (r *Registry) GetDefaultModifiersForType(typeStr string, existingMods []string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	type defaultMod struct {
		key      string
		priority int
	}
	// Pre-allocate with estimated capacity
	defaults := make([]defaultMod, 0, len(r.transformPlugins)+len(r.modifierPlugins))

	// Check TransformPlugins
	for key, p := range r.transformPlugins {
		meta := p.Meta()
		if len(meta.DefaultForTypes) == 0 {
			continue
		}
		// Check if type matches DefaultForTypes
		if !typeAllowed(typeStr, meta.DefaultForTypes) {
			continue
		}
		// Check if disabled by DisabledBy modifier
		if meta.DisabledBy != "" && sliceContains(existingMods, meta.DisabledBy) {
			continue
		}
		// Check if already explicitly specified
		if sliceContains(existingMods, key) {
			continue
		}
		defaults = append(defaults, defaultMod{key: key, priority: meta.Priority})
	}

	// Check ModifierPlugins (e.g., nonempty)
	for key, p := range r.modifierPlugins {
		pm, ok := p.(PluginMeta)
		if !ok {
			continue
		}
		meta := pm.Meta()
		if len(meta.DefaultForTypes) == 0 {
			continue
		}
		// Check if type matches DefaultForTypes
		if !typeAllowed(typeStr, meta.DefaultForTypes) {
			continue
		}
		// Check if disabled by DisabledBy modifier
		if meta.DisabledBy != "" && sliceContains(existingMods, meta.DisabledBy) {
			continue
		}
		// Check if already explicitly specified
		if sliceContains(existingMods, key) {
			continue
		}
		defaults = append(defaults, defaultMod{key: key, priority: meta.Priority})
	}

	// Sort by priority (higher first)
	slices.SortFunc(defaults, func(a, b defaultMod) int {
		return cmp.Compare(b.priority, a.priority)
	})

	result := make([]string, len(defaults))
	for i, d := range defaults {
		result[i] = d.key
	}
	return result
}

// GetDefaultModifiersForType returns modifier keys for the given type from the global registry.
func GetDefaultModifiersForType(typeStr string, existingMods []string) []string {
	return defaultRegistry.GetDefaultModifiersForType(typeStr, existingMods)
}

// GetAllDisablers returns the sorted set of all DisabledBy keys across registered
// transform and modifier plugins. These keys can be used in optval tags to suppress
// automatic modifier application (e.g. "notrim" disables automatic "trimspaces").
func (r *Registry) GetAllDisablers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]bool)

	// From TransformPlugins
	for _, p := range r.transformPlugins {
		if disabler := p.Meta().DisabledBy; disabler != "" {
			seen[disabler] = true
		}
	}

	// From ModifierPlugins
	for _, p := range r.modifierPlugins {
		pm, ok := p.(PluginMeta)
		if !ok {
			continue
		}
		if disabler := pm.Meta().DisabledBy; disabler != "" {
			seen[disabler] = true
		}
	}

	result := make([]string, 0, len(seen))
	for k := range seen {
		result = append(result, k)
	}
	slices.Sort(result)
	return result
}

// GetAllDisablers returns all DisabledBy keys from the global registry.
func GetAllDisablers() []string {
	return defaultRegistry.GetAllDisablers()
}

// sliceContains checks if a string slice contains a value.
func sliceContains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
