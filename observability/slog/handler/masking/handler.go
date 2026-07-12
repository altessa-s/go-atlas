// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking

import (
	"context"
	"encoding"
	"fmt"
	"log/slog"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/observability/slog/handler/internal/base"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// maxMaskDescentDepth bounds reflection-based descent into [slog.KindAny]
// values. A cyclic struct (rare but legal in Go) would otherwise loop
// forever; even an honest deep tree would explode CPU. The limit is
// generous compared to realistic log payloads (~3–5 levels for typical
// API requests/responses); beyond it the value is rendered as-is via
// fmt.Sprint so masking failures cannot crash the logger.
const maxMaskDescentDepth = 10

// maxPathCacheEntries caps the per-handler memoization map for masked
// field paths. Once the cap is reached the cache is atomically swapped
// for a fresh empty map (epoch-style reset) so an attacker driving
// dynamic group names (request IDs, tenant slugs, header names) into
// the slog tree cannot grow the cache for the lifetime of the process.
// 4096 is enough to absorb the steady-state working set of any
// realistic service while keeping peak memory bounded.
const maxPathCacheEntries = 4096

// Handler wraps a [slog.Handler] to mask sensitive fields based on configuration.
// Field matching is case-insensitive. Safe for concurrent use.
type Handler struct {
	base.Base
	opts            *options
	lowercaseFields *coremaps.ImmutableMap[string, MaskFunc] // for case-insensitive matching
	patterns        []compiledPattern
	// pathCache memoizes mask resolution per field path. Stored as an
	// atomic.Pointer[sync.Map] so [Handler.bumpPathCache] can swap in a
	// fresh empty map (epoch reset) once [maxPathCacheEntries] is
	// reached — preventing unbounded growth under dynamic group names.
	pathCache      atomic.Pointer[sync.Map]
	pathCacheCount atomic.Int64
	hasPatterns    bool
	// typeVerdicts memoizes, per reflect.Type, whether a KindAny value of
	// that type can transitively expose a field name matching a configured
	// mask. Shared across derived handlers (the mask configuration is
	// shared too); bounded by the program's type universe, so no epoch
	// reset is needed. Consulted only when typeSkip is true.
	typeVerdicts *sync.Map
	// typeSkip enables the static type verdict. Regex patterns can match
	// arbitrary group prefixes, which the per-type analysis cannot see, so
	// the skip is sound only for fields-only configurations.
	typeSkip bool
}

type compiledPattern struct {
	re   *regexp.Regexp
	mask MaskFunc
}

// NewHandler creates a masking handler wrapping inner. Panics if inner is nil.
//
// Example:
//
//	h := masking.NewHandler(slog.NewJSONHandler(os.Stdout, nil), masking.WithDefaults())
//	logger := slog.New(h)
func NewHandler(inner slog.Handler, opts ...Option) slog.Handler {
	o := newOptions(opts...)

	// Ensure we have a default mask
	if o.defaultMask == nil {
		o.defaultMask = FullMask()
	}

	h := &Handler{
		Base: base.NewBase(inner),
		opts: o,
	}
	// Seed the path cache with an empty map; subsequent epoch resets
	// allocate a fresh one and swap it in atomically.
	h.pathCache.Store(&sync.Map{})

	// Pre-compute lowercase fields for case-insensitive matching
	if !o.caseSensitive {
		tmp := make(map[string]MaskFunc, len(o.fields))
		for k, v := range o.fields {
			tmp[corestrings.InternLowerString(k)] = v
		}
		h.lowercaseFields = coremaps.NewImmutableMap(tmp)
	}

	// Check if we have patterns
	if len(o.patterns) > 0 {
		h.patterns = make([]compiledPattern, 0, len(o.patterns))
		for _, p := range o.patterns {
			if p.Pattern == "" || p.Mask == nil {
				continue
			}
			re, err := regexp.Compile(p.Pattern)
			if err != nil {
				continue
			}
			h.patterns = append(h.patterns, compiledPattern{re: re, mask: p.Mask})
		}
	}
	h.hasPatterns = len(h.patterns) > 0
	h.typeSkip = !h.hasPatterns
	h.typeVerdicts = &sync.Map{}

	return h
}

// Enabled reports whether the inner handler handles records at this level.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.Base.Enabled(ctx, level)
}

// Handle masks sensitive fields and passes the record to the inner handler.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	// Fast path: if no masking configured, skip processing
	if len(h.opts.fields) == 0 && !h.hasPatterns {
		return h.Inner().Handle(ctx, r)
	}

	// Clone the record to avoid modifying the original
	masked := base.CloneRecord(r)

	// Process and mask attributes
	groups := h.Groups()
	r.Attrs(func(a slog.Attr) bool {
		maskedAttr := h.maskAttribute(a, groups)
		masked.AddAttrs(maskedAttr)
		return true
	})

	return h.Inner().Handle(ctx, masked)
}

// WithAttrs returns a new Handler with the given attributes, masking sensitive ones.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Mask attributes before passing to inner handler
	groups := h.Groups()
	maskedAttrs := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		maskedAttrs = append(maskedAttrs, h.maskAttribute(attr, groups))
	}

	cloned := &Handler{
		Base:            h.WithAttrsBase(maskedAttrs),
		opts:            h.opts,
		lowercaseFields: h.lowercaseFields,
		patterns:        h.patterns,
		hasPatterns:     h.hasPatterns,
		typeVerdicts:    h.typeVerdicts,
		typeSkip:        h.typeSkip,
	}
	// Each derived handler gets its own bounded path cache. Sharing the
	// parent's cache would let derived handlers steal cap from each
	// other; isolating them keeps the maxPathCacheEntries bound a
	// per-handler invariant.
	cloned.pathCache.Store(&sync.Map{})
	return cloned
}

// WithGroup returns a new Handler with the given group name.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	cloned := &Handler{
		Base:            h.WithGroupBase(name),
		opts:            h.opts,
		lowercaseFields: h.lowercaseFields,
		patterns:        h.patterns,
		hasPatterns:     h.hasPatterns,
		typeVerdicts:    h.typeVerdicts,
		typeSkip:        h.typeSkip,
	}
	cloned.pathCache.Store(&sync.Map{})
	return cloned
}

// maskAttribute recursively masks an attribute based on configuration
func (h *Handler) maskAttribute(attr slog.Attr, groups []string) slog.Attr {
	// Skip empty attributes
	if attr.Equal(slog.Attr{}) {
		return attr
	}

	// Handle groups recursively. Group keys themselves are never
	// mask-checked (only members are), so the field path is built after
	// this branch. slices.Concat gives the members a fresh backing array —
	// a plain append could let sibling subtrees share and overwrite the
	// same storage mid-walk.
	if attr.Value.Kind() == slog.KindGroup {
		groupAttrs := attr.Value.Group()
		maskedGroup := make([]slog.Attr, 0, len(groupAttrs))
		childGroups := slices.Concat(groups, []string{attr.Key})

		for _, ga := range groupAttrs {
			maskedGroup = append(maskedGroup, h.maskAttribute(ga, childGroups))
		}

		return slog.Group(attr.Key, attrsToAny(maskedGroup)...)
	}

	// Check if field should be masked
	fieldPath := h.buildFieldPath(groups, attr.Key)
	if mask := h.getMaskForField(attr.Key, fieldPath); mask != nil {
		return h.applyMask(attr, mask)
	}

	// Top-level field name didn't match — but a KindAny value can hide
	// a struct/map/slice with sensitive subfields (`payload.password`,
	// `request.token`). Descend via reflection and rebuild the value
	// as slog.GroupValue with matched leaves masked. Without this the
	// original implementation flattened the value via fmt.Sprint and
	// every nested secret leaked verbatim.
	if attr.Value.Kind() == slog.KindAny {
		if masked, ok := h.descendIntoAny(attr.Key, attr.Value, groups); ok {
			return masked
		}
	}

	return attr
}

// descendIntoAny walks a KindAny attribute via reflection and returns a
// new attribute with sensitive subfields masked. The bool result is
// false when descent is impossible (nil value, primitive type, or a
// well-known atomic type like time.Time) — the caller should keep the
// original attribute in that case.
func (h *Handler) descendIntoAny(key string, value slog.Value, groups []string) (slog.Attr, bool) {
	raw := value.Any()
	if raw == nil {
		return slog.Attr{}, false
	}
	// Fields-only configurations can prove statically that a type's
	// transitive field names never match a mask — skip the reflection walk
	// and let the value render natively, exactly like the no-masking fast
	// path in Handle.
	if h.typeSkip && !h.typeCanMatch(reflect.TypeOf(raw)) {
		return slog.Attr{}, false
	}
	walked, walkedOK := h.walkAny(reflect.ValueOf(raw), append(groups, key), 0)
	if !walkedOK {
		return slog.Attr{}, false
	}
	return slog.Attr{Key: key, Value: walked}, true
}

// typeCanMatch reports whether a value of type t can transitively expose a
// field name (or dynamic map key) matching a configured mask field. Verdicts
// are memoized per type; the type universe of logged values is bounded by
// the program's code, so the cache cannot grow unbounded.
func (h *Handler) typeCanMatch(t reflect.Type) bool {
	if v, ok := h.typeVerdicts.Load(t); ok {
		return v.(bool) //nolint:errcheck // only bools are stored
	}
	verdict := h.computeTypeCanMatch(t, make(map[reflect.Type]bool))
	h.typeVerdicts.Store(t, verdict)
	return verdict
}

// computeTypeCanMatch is the uncached DFS behind [Handler.typeCanMatch]. It
// mirrors walkAny's descent rules: pointers unwrap, atomic types stop,
// structs/slices recurse, string-keyed maps expose dynamic keys the static
// analysis cannot see (conservatively a match). Interfaces hide the dynamic
// type, so they are conservatively a match too. The visiting set breaks
// recursive types: revisiting a type cannot add names beyond its first pass.
func (h *Handler) computeTypeCanMatch(t reflect.Type, visiting map[reflect.Type]bool) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() == reflect.Interface {
		return true
	}
	if isAtomicType(t) {
		return false
	}
	if visiting[t] {
		return false
	}

	switch t.Kind() {
	case reflect.Struct:
		visiting[t] = true
		for i := range t.NumField() {
			ft := t.Field(i)
			if !ft.IsExported() {
				continue
			}
			name := structFieldName(ft)
			if name == "" { // json:"-"
				continue
			}
			if h.fieldNameCanMatch(name) || h.computeTypeCanMatch(ft.Type, visiting) {
				return true
			}
		}
		return false
	case reflect.Map:
		// Non-string-keyed maps are never walked (walkMap passes them
		// through untouched), so only string keys count.
		return t.Key().Kind() == reflect.String
	case reflect.Slice, reflect.Array:
		return h.computeTypeCanMatch(t.Elem(), visiting)
	default:
		return false
	}
}

// fieldNameCanMatch reports whether a walked field name can satisfy either
// lookup in getMaskForField: the exact-name check (key == name) or the
// dotted-path check — a path's last segment is always the field name, so a
// configured key can only match when it ends with "."+name.
func (h *Handler) fieldNameCanMatch(name string) bool {
	if h.opts.caseSensitive {
		suffix := "." + name
		for k := range h.opts.fields {
			if k == name || strings.HasSuffix(k, suffix) {
				return true
			}
		}
		return false
	}
	if h.lowercaseFields == nil {
		return false
	}
	lower := corestrings.InternLowerString(name)
	suffix := "." + lower
	for k := range h.lowercaseFields.All() {
		if k == lower || strings.HasSuffix(k, suffix) {
			return true
		}
	}
	return false
}

// walkAny is the reflect-driven recursive walker used by descendIntoAny.
// Returns (slog.Value, true) when descent rebuilt the value, or
// (zero, false) when the value is atomic (and the caller should keep
// the original). Depth is bounded by [maxMaskDescentDepth].
func (h *Handler) walkAny(rv reflect.Value, groups []string, depth int) (slog.Value, bool) {
	if !rv.IsValid() || depth >= maxMaskDescentDepth {
		return slog.Value{}, false
	}

	// Unwrap pointers/interfaces. nil pointer/interface short-circuits.
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return slog.Value{}, false
		}
		rv = rv.Elem()
	}

	// Atomic types: well-known stdlib types that have their own canonical
	// representation and MUST NOT be flattened into a slog group. Adding
	// to this list is the migration path when new types surface in audits.
	if isAtomicType(rv.Type()) {
		return slog.Value{}, false
	}

	switch rv.Kind() {
	case reflect.Struct:
		return h.walkStruct(rv, groups, depth)
	case reflect.Map:
		return h.walkMap(rv, groups, depth)
	case reflect.Slice, reflect.Array:
		return h.walkSlice(rv, groups, depth)
	default:
		// Scalars, pointers, channels, funcs, interfaces, and unsafe
		// pointers do not have walkable substructure. Returning
		// (zero, false) signals the caller to pass the value through
		// to the underlying handler unchanged.
		return slog.Value{}, false
	}
}

// walkStruct walks the exported fields of a struct and rebuilds the
// value as a slog group with masked fields where rules match.
func (h *Handler) walkStruct(rv reflect.Value, groups []string, depth int) (slog.Value, bool) {
	t := rv.Type()
	attrs := make([]slog.Attr, 0, rv.NumField())
	for i := range rv.NumField() {
		ft := t.Field(i)
		if !ft.IsExported() {
			continue
		}
		name := structFieldName(ft)
		if name == "" { // json:"-"
			continue
		}
		attrs = append(attrs, h.attrForField(name, rv.Field(i), groups, depth))
	}
	// No exported fields: nothing to mask or rebuild. An empty group would
	// replace the value, and slog drops empty groups — so the attribute would
	// vanish. Signal "not rebuilt" so the caller keeps the original and lets the
	// downstream handler render it canonically (e.g. an error via Error()).
	if len(attrs) == 0 {
		return slog.Value{}, false
	}
	return slog.GroupValue(attrs...), true
}

// walkMap walks a string-keyed map. Non-string-keyed maps are left alone
// — slog's JSON handler renders them via Sprint anyway, and walking them
// would require coercing keys to strings (lossy and surprising).
func (h *Handler) walkMap(rv reflect.Value, groups []string, depth int) (slog.Value, bool) {
	if rv.Type().Key().Kind() != reflect.String {
		return slog.Value{}, false
	}
	attrs := make([]slog.Attr, 0, rv.Len())
	for iter := rv.MapRange(); iter.Next(); {
		key := iter.Key().String()
		attrs = append(attrs, h.attrForField(key, iter.Value(), groups, depth))
	}
	// Same empty-group hazard as walkStruct: an empty map rebuilds to an empty
	// group, which slog omits — the attribute would vanish. Signal "not rebuilt"
	// so the caller keeps the original and it renders canonically (e.g. map[]).
	if len(attrs) == 0 {
		return slog.Value{}, false
	}
	return slog.GroupValue(attrs...), true
}

// walkSlice walks slice/array elements; sensitive subfields inside
// element structs/maps are still caught because the parent group path
// is preserved. The index is used as the attr key so consumers can map
// each masked leaf back to its original position.
func (h *Handler) walkSlice(rv reflect.Value, groups []string, depth int) (slog.Value, bool) {
	attrs := make([]slog.Attr, 0, rv.Len())
	for i := range rv.Len() {
		elem := rv.Index(i)
		key := sliceIndexKey(i)
		if nested, ok := h.walkAny(elem, groups, depth+1); ok {
			attrs = append(attrs, slog.Attr{Key: key, Value: nested})
			continue
		}
		// Atomic element — pass through as-is via slog.AnyValue.
		attrs = append(attrs, slog.Attr{Key: key, Value: slog.AnyValue(elem.Interface())})
	}
	// Same empty-group hazard as walkStruct: an empty slice/array rebuilds to an
	// empty group, which slog omits — the attribute would vanish. Signal "not
	// rebuilt" so the caller keeps the original and it renders canonically (e.g. []).
	if len(attrs) == 0 {
		return slog.Value{}, false
	}
	return slog.GroupValue(attrs...), true
}

// sliceIndexKeys pre-renders the "[i]" keys walkSlice assigns to elements so
// the per-element hot path avoids fmt.Sprintf. 64 covers realistic log
// payloads; larger indexes fall back to allocating.
var sliceIndexKeys = func() (keys [64]string) {
	for i := range keys {
		keys[i] = "[" + strconv.Itoa(i) + "]"
	}
	return keys
}()

func sliceIndexKey(i int) string {
	if i < len(sliceIndexKeys) {
		return sliceIndexKeys[i]
	}
	return "[" + strconv.Itoa(i) + "]"
}

// attrForField checks whether the supplied field matches a mask rule
// and, if so, applies it to the stringified value. Otherwise it
// recurses via walkAny (or passes the leaf through unchanged).
func (h *Handler) attrForField(name string, field reflect.Value, parentGroups []string, depth int) slog.Attr {
	fieldPath := h.buildFieldPath(parentGroups, name)
	if mask := h.getMaskForField(name, fieldPath); mask != nil {
		// Match: mask the stringified value. Use Interface() so types
		// with custom Stringer get their own representation before
		// masking — keeps masked output consistent with downstream
		// log formatting.
		var v any
		if field.CanInterface() {
			v = field.Interface()
		}
		return slog.String(name, mask(fmt.Sprint(v)))
	}
	// slices.Concat allocates a fresh backing array — recursive walks
	// otherwise share parentGroups' storage and a sibling branch can
	// overwrite an earlier child's path mid-walk.
	childGroups := slices.Concat(parentGroups, []string{name})
	if nested, ok := h.walkAny(field, childGroups, depth+1); ok {
		return slog.Attr{Key: name, Value: nested}
	}
	// Leaf: preserve the original value as-is so structured handlers
	// (JSON, OTLP) render it natively.
	if field.CanInterface() {
		return slog.Any(name, field.Interface())
	}
	return slog.Attr{Key: name}
}

// structFieldName returns the name to use for a struct field in the
// masked output. Honors `json` tags (so callers who already JSON-tagged
// their structs get the masking matching they expect) but falls back to
// the Go field name.
func structFieldName(ft reflect.StructField) string {
	tag, ok := ft.Tag.Lookup("json")
	if !ok {
		return ft.Name
	}
	if i := strings.IndexByte(tag, ','); i >= 0 {
		tag = tag[:i]
	}
	switch tag {
	case "":
		return ft.Name
	case "-":
		return ""
	default:
		return tag
	}
}

// timeType / textMarshalerType / stringerType / logValuerType pre-cache
// the reflect.Type values we compare against in [isAtomicType] so the
// hot path does not allocate. logValuer is the slog way to control a
// value's representation; respecting it keeps custom types intact.
var (
	timeType          = reflect.TypeFor[time.Time]()
	durationType      = reflect.TypeFor[time.Duration]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
	logValuerType     = reflect.TypeFor[slog.LogValuer]()
	byteSliceType     = reflect.TypeFor[[]byte]()
)

// isAtomicType reports whether reflect descent must stop at this type.
// Types that ship their own canonical representation (time.Time,
// encoding.TextMarshaler implementations, slog.LogValuer
// implementations, []byte) should be rendered as-is by the downstream
// handler, not flattened into a slog group.
func isAtomicType(t reflect.Type) bool {
	switch t {
	case timeType, durationType, byteSliceType:
		return true
	}
	if t.Implements(logValuerType) || reflect.PointerTo(t).Implements(logValuerType) {
		return true
	}
	if t.Implements(textMarshalerType) || reflect.PointerTo(t).Implements(textMarshalerType) {
		return true
	}
	// fmt.Stringer is intentionally NOT treated as atomic: it is a display
	// hint, not a canonical encoding. A struct with sensitive fields that also
	// implements String() must still be descended into and masked field by
	// field; otherwise its String() output would be logged verbatim, bypassing
	// the masking handler. Only slog.LogValuer (which explicitly controls log
	// representation) and encoding.TextMarshaler stop descent here.
	return false
}

// getMaskForField uses pre-computed data for faster lookups
func (h *Handler) getMaskForField(fieldName, fieldPath string) MaskFunc {
	cache := h.pathCache.Load()

	// Try cache first
	if cached, ok := cache.Load(fieldPath); ok {
		if mask, ok := cached.(MaskFunc); ok {
			return mask
		}
	}

	var mask MaskFunc

	// Check nested field paths first if enabled (more specific)
	if h.opts.maskNestedFields && fieldPath != fieldName {
		if h.opts.caseSensitive {
			mask = h.opts.fields[fieldPath]
		} else if h.lowercaseFields != nil {
			mask, _ = h.lowercaseFields.Get(corestrings.InternLowerString(fieldPath))
		}
	}

	// Check exact field matches if no nested match
	if mask == nil {
		if h.opts.caseSensitive {
			mask = h.opts.fields[fieldName]
		} else if h.lowercaseFields != nil {
			mask, _ = h.lowercaseFields.Get(corestrings.InternLowerString(fieldName))
		}
	}

	// Check pattern matches (currently not working in original implementation)
	if mask == nil && h.hasPatterns {
		// Prefer the full field path when nested masking is enabled.
		target := fieldName
		if h.opts.maskNestedFields && fieldPath != fieldName {
			target = fieldPath
		}

		for i := range h.patterns {
			if h.patterns[i].re.MatchString(target) {
				mask = h.patterns[i].mask
				break
			}
		}
	}

	// Only cache POSITIVE matches. Caching every nil result would let
	// an attacker driving dynamic group/field names (request IDs,
	// tenant slugs, header names) grow the cache without bound — those
	// paths are exactly the ones that will always miss. Positive hits
	// represent the configured field universe and are naturally
	// bounded by the user's mask list.
	if mask != nil {
		// Epoch reset: once we hit the cap, swap the whole map for a
		// fresh empty one. Coarser than LRU but allocation-free on the
		// hot path and bounds the worst-case memory footprint hard.
		if h.pathCacheCount.Load() >= maxPathCacheEntries {
			fresh := &sync.Map{}
			if h.pathCache.CompareAndSwap(cache, fresh) {
				h.pathCacheCount.Store(0)
				cache = fresh
			} else {
				cache = h.pathCache.Load()
			}
		}
		if _, loaded := cache.LoadOrStore(fieldPath, mask); !loaded {
			h.pathCacheCount.Add(1)
		}
	}

	return mask
}

// buildFieldPath creates the full path for nested fields
func (h *Handler) buildFieldPath(groups []string, field string) string {
	if !h.opts.maskNestedFields || len(groups) == 0 {
		return field
	}

	// Use strings.Builder for better performance
	var sb strings.Builder
	// Pre-calculate capacity
	capacity := len(field)
	for _, g := range groups {
		capacity += len(g) + 1
	}
	sb.Grow(capacity)

	// Build path
	for _, g := range groups {
		sb.WriteString(g)
		sb.WriteByte('.')
	}
	sb.WriteString(field)

	return sb.String()
}

// applyMask applies the masking function to an attribute value
func (h *Handler) applyMask(attr slog.Attr, mask MaskFunc) slog.Attr {
	switch attr.Value.Kind() {
	case slog.KindString:
		return slog.String(attr.Key, mask(attr.Value.String()))

	case slog.KindInt64, slog.KindUint64, slog.KindFloat64, slog.KindBool:
		// Convert to string, mask, and keep as string
		str := fmt.Sprint(attr.Value.Any())
		return slog.String(attr.Key, mask(str))

	case slog.KindDuration:
		return slog.String(attr.Key, mask(attr.Value.Duration().String()))

	case slog.KindTime:
		return slog.String(attr.Key, mask(attr.Value.Time().String()))

	case slog.KindAny:
		// Top-level key matched: mask the entire value as a single
		// string. Per-subfield masking for KindAny is handled at the
		// maskAttribute level via descendIntoAny / walkAny, which is
		// where nested matches inside structs/maps/slices are found.
		str := fmt.Sprint(attr.Value.Any())
		return slog.String(attr.Key, mask(str))

	default:
		// For unknown types, convert to string and mask
		str := fmt.Sprint(attr.Value.Any())
		return slog.String(attr.Key, mask(str))
	}
}

// attrsToAny converts attributes to []any for slog.Group
func attrsToAny(attrs []slog.Attr) []any {
	result := make([]any, len(attrs))
	for i, attr := range attrs {
		result[i] = attr
	}
	return result
}
