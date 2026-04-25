// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"hash/fnv"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/data/cache/lru"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

const (
	// PathSeparator is the delimiter used in dot-separated field paths
	// (e.g. "user.name").
	PathSeparator = "."

	// DefaultEstimatedPathCapacity is the initial slice capacity when collecting
	// paths from a protobuf message via [FromMessage] or [FromSetFields].
	DefaultEstimatedPathCapacity = 24

	// EstimatedFieldsPerPath is the assumed average nesting depth per path,
	// used to estimate map capacity in [FromPaths].
	EstimatedFieldsPerPath = 3

	// MinMapCapacity is the minimum [FieldMask] map capacity to reduce
	// early rehashing.
	MinMapCapacity = 8

	// DefaultToPathsCacheSize is the LRU cache capacity for [FieldMask.ToPaths]
	// results, keyed by a hash of the mask structure.
	DefaultToPathsCacheSize = 256
)

// Global LRU cache for ToPaths results using hashicorp/golang-lru.
// Initialized lazily to avoid panicking in init() for a performance optimization.
var (
	toPathsCacheOnce   sync.Once
	toPathsCacheSize   = DefaultToPathsCacheSize
	globalToPathsCache lru.Cacher[uint64, []string]
)

func getToPathsCache() lru.Cacher[uint64, []string] {
	toPathsCacheOnce.Do(func() {
		if toPathsCacheSize <= 0 {
			return
		}
		cache, err := lru.NewCache[uint64, []string](toPathsCacheSize)
		if err != nil {
			// Best-effort: caching is an optimization, so degrade gracefully.
			return
		}
		globalToPathsCache = cache
	})
	return globalToPathsCache
}

// FieldMask represents a hierarchical field mask as a nested map structure.
// Keys are field names; nil values indicate leaf fields (keep/prune the entire subtree).
// All methods are safe for nil or empty receivers and produce deterministic output.
type FieldMask map[string]FieldMask

// hash computes a hash for the FieldMask for caching.
func (msk FieldMask) hash() uint64 {
	h := fnv.New64a()
	// Sort keys for consistent hashing
	keys := slices.Sorted(maps.Keys(msk))

	for _, k := range keys {
		_, _ = h.Write([]byte(k))
		_, _ = h.Write([]byte{0}) // Separator
		if nested := msk[k]; nested != nil {
			// Recursively hash nested masks
			nestedHash := nested.hash()
			//nolint:mnd // Standard uint64 to bytes conversion
			_, _ = h.Write([]byte{byte(nestedHash), byte(nestedHash >> 8), byte(nestedHash >> 16), byte(nestedHash >> 24),
				byte(nestedHash >> 32), byte(nestedHash >> 40), byte(nestedHash >> 48), byte(nestedHash >> 56)})
		}
	}
	return h.Sum64()
}

// estimateMapCapacity estimates required map capacity based on path count.
func estimateMapCapacity(pathCount int) int {
	if pathCount == 0 {
		return 0
	}
	// Estimate: pathCount * average depth per path
	capacity := pathCount * EstimatedFieldsPerPath
	// Use minimum capacity to avoid small map rehashing
	return max(capacity, MinMapCapacity)
}

// estimatePathCount recursively estimates the number of paths in the mask.
func (msk FieldMask) estimatePathCount() int {
	count := 0
	for _, nested := range msk {
		if nested == nil {
			count++
		} else {
			count += nested.estimatePathCount()
		}
	}
	return count
}

// FromPaths creates a FieldMask from dot-separated field paths.
// Empty paths are filtered out. Creates hierarchical nested structure.
//
// Example:
//
//	mask := fieldmask.FromPaths("user.name", "user.address.city")
func FromPaths(paths ...string) FieldMask {
	// Filter out empty paths to avoid processing them
	paths = slices.Collect(coreslices.Filter(paths, func(i string) bool {
		return i != ""
	}))

	if len(paths) == 0 {
		return make(FieldMask)
	}

	// Pre-allocate map with estimated capacity based on number of paths
	capacity := estimateMapCapacity(len(paths))
	mask := make(FieldMask, capacity)

	for _, path := range paths {
		fromPath(mask, path)
	}

	return mask
}

// FromProtoFieldMask creates a FieldMask from a protobuf FieldMask.
//
// Example:
//
//	mask := fieldmask.FromProtoFieldMask(pbMask)
func FromProtoFieldMask(protoMask *fieldmaskpb.FieldMask) FieldMask {
	return FromPaths(protoMask.GetPaths()...)
}

// FromMessage creates a FieldMask or []string for all fields defined in the message schema.
// Returns all paths regardless of whether fields are set.
func FromMessage[T interface{ []string | FieldMask }](msg proto.Message) T {
	return fromMessage[T](msg, false)
}

// FromSetFields creates a FieldMask or []string for only set (non-nil/non-zero) fields.
// Recursively traverses nested structures.
func FromSetFields[T interface{ []string | FieldMask }](msg proto.Message) T {
	return fromMessage[T](msg, true)
}

func fromMessage[T interface{ []string | FieldMask }](msg proto.Message, onlySet bool) T {
	var tmp T
	if msg == nil {
		return tmp
	}

	pathsList := make([]string, 0, DefaultEstimatedPathCapacity)
	buildPathsRecursive(msg.ProtoReflect(), "", &pathsList, onlySet)

	switch any(tmp).(type) {
	case []string:
		slices.Sort(pathsList)
		tmp = any(pathsList).(T) //nolint:errcheck
	case FieldMask:
		tmp = any(FromPaths(pathsList...)).(T) //nolint:errcheck
	}
	return tmp
}

// PrunePaths removes fields from the message matching the given paths.
// Returns the original message if no paths provided.
//
// Example:
//
//	fieldmask.PrunePaths(msg, "user.password", "user.secret")
func PrunePaths(msg proto.Message, paths ...string) proto.Message {
	if len(paths) == 0 {
		return msg // No paths to prune, return original message
	}
	fm := FromPaths(paths...)
	fm.Prune(msg)

	return msg
}

// ToProtoFieldMask converts the FieldMask to a protobuf FieldMask.
func (msk FieldMask) ToProtoFieldMask() *fieldmaskpb.FieldMask {
	return &fieldmaskpb.FieldMask{Paths: msk.ToPaths()}
}

// ToPaths converts the FieldMask to a flat list of dot-separated paths.
// Results are cached using LRU cache for improved performance.
func (msk FieldMask) ToPaths() []string {
	if len(msk) == 0 {
		return []string{}
	}

	// Try to get from cache first
	hash := msk.hash()
	if cache := getToPathsCache(); cache != nil {
		if cached, ok := cache.Get(hash); ok {
			// Return a copy to prevent modifications to cached data
			result := make([]string, len(cached))
			copy(result, cached)
			return result
		}
	}

	// Cache miss - compute paths
	capacity := msk.estimatePathCount()
	paths := make([]string, 0, capacity)
	msk.buildPaths("", &paths)

	// Ensure deterministic output order (map iteration order is randomized).
	slices.Sort(paths)

	// Store a copy in cache to prevent external modifications
	pathsCopy := make([]string, len(paths))
	copy(pathsCopy, paths)
	if cache := getToPathsCache(); cache != nil {
		cache.Put(hash, pathsCopy)
	}

	return paths
}

// Union combines two masks into one containing all fields from both.
// Fields in both masks are recursively combined.
//
// Example:
//
//	combined := mask1.Union(mask2)
func (msk FieldMask) Union(other FieldMask) FieldMask {
	if len(msk) == 0 {
		return other.Clone()
	} else if len(other) == 0 {
		return msk.Clone()
	}

	// Pre-allocate result map with capacity for union of both masks
	capacity := len(msk) + len(other)
	result := make(FieldMask, capacity)

	// Copy all fields from the first mask
	for field, nested := range msk {
		if nested == nil {
			result[field] = nil
		} else {
			result[field] = nested.Clone()
		}
	}

	// Add or merge fields from the second mask
	for field, otherNested := range other {
		resultNested, exists := result[field]
		switch {
		case !exists:
			// Field only exists in the second mask
			if otherNested == nil {
				result[field] = nil
				continue
			}
			result[field] = otherNested.Clone()
		case resultNested == nil || otherNested == nil:
			// If either mask has a nil value for this field, it's a leaf field
			result[field] = nil
		default:
			result[field] = resultNested.Union(otherNested)
		}
	}

	return result
}

// Intersection creates a mask containing only fields that exist in both masks.
// Fields are recursively intersected.
//
// Example:
//
//	common := mask1.Intersection(mask2)
func (msk FieldMask) Intersection(other FieldMask) FieldMask {
	if len(msk) == 0 || len(other) == 0 {
		return make(FieldMask)
	}

	// Create a new mask for the result
	result := make(FieldMask)

	for field, nested := range msk {
		otherNested, exists := other[field]
		if !exists {
			// The field doesn't exist in the other mask
			continue
		}

		switch {
		case nested == nil && otherNested == nil:
			// Both are leaves (or represent full subtrees implicitly)
			result[field] = nil
		case nested == nil:
			result[field] = otherNested.Clone()
		case otherNested == nil:
			result[field] = nested.Clone()
		default:
			// Both masks specify nested fields, find their intersection recursively.
			intersection := nested.Intersection(otherNested)
			// Only add the field to the result if the sub-intersection is non-empty.
			if len(intersection) > 0 {
				result[field] = intersection
			}
			// If intersection is empty, do nothing (correct for intersection).
		}
	}

	return result
}

// Difference returns paths in the receiver but NOT in the other mask.
// Does not support "all except specific parts" for subtrees.
//
// Example:
//
//	diff := mask1.Difference(mask2)
func (msk FieldMask) Difference(other FieldMask) FieldMask {
	if len(msk) == 0 {
		return FieldMask{} // Difference from empty set is empty
	} else if len(other) == 0 {
		return msk.Clone() // Difference from empty set is the original set
	}

	result := make(FieldMask)

	for field, nested1 := range msk {
		nested2, existsInOther := other[field]

		if !existsInOther && nested1 == nil {
			result[field] = nil
			continue
		} else if !existsInOther {
			result[field] = nested1.Clone() // Clone needed for nested maps
			continue
		}

		if nested1 != nil && nested2 != nil {
			// Both are non-nil, recurse
			subDifference := nested1.Difference(nested2)
			if len(subDifference) > 0 {
				result[field] = subDifference
			}
			continue
		}
	}
	return result
}

// Clone creates a deep copy of the FieldMask.
func (msk FieldMask) Clone() FieldMask {
	if len(msk) == 0 {
		return make(FieldMask)
	}

	result := make(FieldMask, len(msk))
	for field, nested := range msk {
		if nested == nil {
			result[field] = nil
		} else {
			result[field] = nested.Clone()
		}
	}

	return result
}

// IsEmpty returns true if the field mask contains no fields.
func (msk FieldMask) IsEmpty() bool {
	return len(msk) == 0
}

// Contains checks if a dot-separated path exists in the FieldMask.
//
// Example:
//
//	exists := mask.Contains("user.address.city")
func (msk FieldMask) Contains(path string) bool {
	if path == "" || len(msk) == 0 {
		return false
	}

	parts := strings.Split(path, PathSeparator)
	currentLevel := msk

	for i, part := range parts {
		if currentLevel == nil {
			// We need to go deeper, but the mask level is nil (leaf) or non-existent.
			return false
		}

		nested, specificExists := currentLevel[part]

		if !specificExists {
			// The specific part does not exist at this level
			return false
		}

		// If this is the last part of the path, we've found a match
		if i == len(parts)-1 {
			return true
		}

		// Continue to the next part of the path with the nested mask
		currentLevel = nested
	}

	// Should not be reached if parts is not empty
	return false
}

// Validate checks if all paths exist in the message schema.
// Returns ValidationError if any path is invalid. Validates schema only, not values.
func (msk FieldMask) Validate(msg proto.Message) error {
	if msg == nil {
		return nil
	}
	if len(msk) == 0 {
		return nil
	}

	descriptor := msg.ProtoReflect().Descriptor()
	paths := msk.ToPaths()

	for _, path := range paths {
		if err := validatePath(descriptor, path); err != nil {
			return err
		}
	}

	return nil
}

// Filter keeps only fields present in the mask and clears all others.
// Handles nested messages, lists, and maps.
//
// Example:
//
//	mask.Filter(protoMessage) // Keep only specified fields
func (msk FieldMask) Filter(msg proto.Message) {
	if len(msk) == 0 {
		return
	}
	msk.iterateFilter(msg)
}

// iterateFilter recursively applies the filter mask.
func (msk FieldMask) iterateFilter(msg proto.Message) {
	if s, ok := msg.(*structpb.Struct); ok {
		msk.filterStruct(s)
		return
	}
	if lv, ok := msg.(*structpb.ListValue); ok {
		msk.filterListValue(lv)
		return
	}
	if v, ok := msg.(*structpb.Value); ok {
		msk.filterStructValue(v)
		return
	}
	prf := msg.ProtoReflect()

	prf.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		field := string(fd.Name())

		// Determine the effective mask for this specific field
		nested, exists := msk[field]

		if !exists {
			prf.Clear(fd)
			return true
		}

		effectiveNested := nested

		// Field should be kept. Now check if its contents need filtering.
		if effectiveNested == nil {
			// Keep the entire field/subtree as is (mask is leaf)
			return true
		}

		switch {
		case fd.IsList():
			applyToMessageList(fd, prf.Get(fd).List(), effectiveNested, func(nested FieldMask, m proto.Message) {
				nested.iterateFilter(m)
			})
		case fd.IsMap():
			mm := prf.Get(fd).Map()
			if !mm.IsValid() || mm.Len() == 0 {
				return true // Keep empty map field if map itself is kept
			}

			mapKeyMask := effectiveNested // This is the mask for the map field (e.g., msk["flags"])

			mm.Range(func(k protoreflect.MapKey, mapVal protoreflect.Value) bool {
				keyString := k.String()
				// Look up the string key in the map's specific mask.
				specificValueMask, specificKeyInMapMaskExists := mapKeyMask[keyString]

				if !specificKeyInMapMaskExists {
					mm.Clear(k) // Key not found in the specific map mask, remove entry
					return true
				}

				finalValueMask := specificValueMask
				if finalValueMask == nil {
					return true // Keep value as is (key exists, value mask is nil)
				}

				// Check the kind of the map value using its descriptor
				mapValueDesc := fd.MapValue()
				if mapValueDesc != nil && mapValueDesc.Kind() == protoreflect.MessageKind {
					// Use finalValueMask as the mask for the value message structure
					finalValueMask.iterateFilter(mapVal.Message().Interface())
				}
				return true
			})

		case fd.Kind() == protoreflect.MessageKind:
			// Apply the effectiveNested mask recursively to the sub-message
			effectiveNested.iterateFilter(prf.Get(fd).Message().Interface())
		} // default case: scalars, enums - already kept if shouldKeep is true
		return true // Continue to next field
	})
}

// Prune removes fields present in the mask while keeping others.
// Opposite of Filter. Handles nested messages, lists, and maps.
//
// Example:
//
//	mask.Prune(protoMessage) // Remove specified fields
func (msk FieldMask) Prune(msg proto.Message) {
	if len(msk) == 0 {
		return // Empty mask means prune nothing
	}
	msk.iteratePrune(msg)
}

// iteratePrune recursively applies the prune mask.
func (msk FieldMask) iteratePrune(msg proto.Message) {
	if s, ok := msg.(*structpb.Struct); ok {
		msk.pruneStruct(s)
		return
	}
	if lv, ok := msg.(*structpb.ListValue); ok {
		msk.pruneListValue(lv)
		return
	}
	if v, ok := msg.(*structpb.Value); ok {
		msk.pruneStructValue(v)
		return
	}
	prf := msg.ProtoReflect()

	prf.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		field := string(fd.Name())

		// Determine if this field should be pruned based on the specific mask
		nested, exists := msk[field]

		if !exists {
			return true // Field not in mask, leave it untouched
		}

		pruneMask := nested

		// Field is targeted by the mask. Decide how to prune.
		if pruneMask == nil {
			// Mask is a leaf (e.g., "field"), clear the field entirely.
			prf.Clear(fd)
			return true
		}

		switch {
		case fd.IsList():
			applyToMessageList(fd, prf.Get(fd).List(), pruneMask, func(nested FieldMask, m proto.Message) {
				nested.iteratePrune(m)
			})
		case fd.IsMap():
			mm := prf.Get(fd).Map()
			if !mm.IsValid() || mm.Len() == 0 {
				return true
			} // Empty map field

			// pruneMask now holds the mask for the map keys/values (e.g., msk["flags"])
			mapKeyMask := pruneMask
			mapValueDesc := fd.MapValue() // Get descriptor for map values

			mm.Range(func(k protoreflect.MapKey, mapVal protoreflect.Value) bool {
				keyString := k.String()
				// Look up the string key in the map's specific mask.
				specificValueMask, specificKeyInMapMaskExists := mapKeyMask[keyString]

				if !specificKeyInMapMaskExists {
					return true // This key is not targeted by the prune mask, keep entry.
				}

				finalValuePruneMask := specificValueMask

				// This map entry is targeted. Decide how to prune.
				if finalValuePruneMask == nil {
					// Mask is leaf for this key (e.g., "flags.active"), clear the entry.
					mm.Clear(k)
					return true
				}

				// Prune recursively within the value if it's a message
				if mapValueDesc != nil && mapValueDesc.Kind() == protoreflect.MessageKind {
					// Use finalValuePruneMask as the mask for the value message structure
					finalValuePruneMask.iteratePrune(mapVal.Message().Interface())
				}
				return true
			})

		case fd.Kind() == protoreflect.MessageKind:
			// Apply the pruneMask recursively to the sub-message
			pruneMask.iteratePrune(prf.Get(fd).Message().Interface())
		} // default case: scalars, enums - already cleared if pruneMask was nil
		return true
	})
}

func applyToMessageList(fd protoreflect.FieldDescriptor, lst protoreflect.List, nested FieldMask, apply func(FieldMask, proto.Message)) {
	if lst.Len() == 0 {
		return
	}

	// Only message lists can be traversed.
	if fd.Kind() != protoreflect.MessageKind || fd.Message() == nil {
		return
	}

	// Apply the nested mask to each element.
	// Note: index-specific masks are not supported; nested applies to all elements.
	for i := range lst.Len() {
		apply(nested, lst.Get(i).Message().Interface())
	}
}

// buildPaths recursively traverses the mask and appends full paths.
func (msk FieldMask) buildPaths(prefix string, paths *[]string) {
	for field, nested := range msk {
		currentPath := field
		if prefix != "" {
			currentPath = prefix + PathSeparator + field
		}

		if nested == nil {
			// Leaf node, add the full path
			*paths = append(*paths, currentPath)
		} else {
			// Branch node, recurse deeper
			nested.buildPaths(currentPath, paths)
		}
	}
}

// fromPath parses a dot-separated path and builds nested structure.
func fromPath(mask FieldMask, path string) {
	if path == "" {
		return
	}

	// Find the first dot in the path
	dotIndex := strings.Index(path, PathSeparator)
	if dotIndex == -1 {
		// If no dot is found, this is a leaf field or the end of a branch.
		// Leaf dominates: if a parent leaf is present, nested paths are redundant.
		mask[path] = nil
		return
	}

	// Split the path into the current field and the rest
	field := path[:dotIndex]
	if field == "" {
		// Invalid path format
		return
	}

	rest := path[dotIndex+1:]
	if rest == "" {
		// If there's nothing left after the last dot, treat it as a leaf field.
		mask[field] = nil
		return
	}

	// Get or create the nested mask for this field
	nested, exists := mask[field]
	if exists && nested == nil {
		// Leaf dominates: do not expand into nested mask if the parent field is already a leaf.
		return
	}
	if !exists {
		nested = make(FieldMask)
		mask[field] = nested
	}
	// If 'nested' exists and is not nil, it means it's already a branch, so we use it.

	// Continue parsing the rest of the path recursively
	fromPath(nested, rest)
}

// zeroValueForScalar returns the zero value for a scalar field type.
func zeroValueForScalar(fd protoreflect.FieldDescriptor) protoreflect.Value {
	var zeroValue protoreflect.Value
	switch fd.Kind() { // Kind of the list field determines element kind
	case protoreflect.StringKind:
		zeroValue = protoreflect.ValueOfString("")
	case protoreflect.BoolKind:
		zeroValue = protoreflect.ValueOfBool(false)
	case protoreflect.BytesKind:
		zeroValue = protoreflect.ValueOfBytes([]byte{})
	case protoreflect.DoubleKind:
		zeroValue = protoreflect.ValueOfFloat64(0)
	case protoreflect.FloatKind:
		zeroValue = protoreflect.ValueOfFloat32(0)
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind, protoreflect.EnumKind:
		zeroValue = protoreflect.ValueOfInt32(0)
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		zeroValue = protoreflect.ValueOfInt64(0)
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		zeroValue = protoreflect.ValueOfUint32(0)
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		zeroValue = protoreflect.ValueOfUint64(0)
	default:
	}
	return zeroValue
}

// isFieldSet checks if a field is considered "set" (non-nil/non-zero).
func isFieldSet(msg protoreflect.Message, fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
	switch {
	case fd.IsList():
		return v.List().Len() > 0
	case fd.IsMap():
		return v.Map().Len() > 0
	case fd.HasPresence(): // For fields with optional/presence
		return msg.Has(fd)
	default: // For proto3 scalars without presence - check against zero value
		return !v.Equal(fd.Default())
	}
}

// structMessageFullName is the full protobuf name for google.protobuf.Struct.
const structMessageFullName = protoreflect.FullName("google.protobuf.Struct")

// isStructWellKnownType returns true when fd points to a google.protobuf.Struct
// message field. Struct needs special handling: its dynamic keys are navigable
// by field mask operations, unlike opaque types (Value, ListValue).
func isStructWellKnownType(fd protoreflect.FieldDescriptor) bool {
	return fd.Kind() == protoreflect.MessageKind && fd.Message().FullName() == structMessageFullName
}

// isStructWellKnownMessage returns true when md is the google.protobuf.Struct descriptor.
func isStructWellKnownMessage(md protoreflect.MessageDescriptor) bool {
	return md != nil && md.FullName() == structMessageFullName
}

// listValueMessageFullName is the full protobuf name for google.protobuf.ListValue.
const listValueMessageFullName = protoreflect.FullName("google.protobuf.ListValue")

// isListValueWellKnownType returns true when fd points to a google.protobuf.ListValue
// message field. ListValue needs special handling: the mask is applied to all elements
// of the list that are Struct or nested ListValue values.
func isListValueWellKnownType(fd protoreflect.FieldDescriptor) bool {
	return fd.Kind() == protoreflect.MessageKind && fd.Message().FullName() == listValueMessageFullName
}

// isListValueWellKnownMessage returns true when md is the google.protobuf.ListValue descriptor.
func isListValueWellKnownMessage(md protoreflect.MessageDescriptor) bool {
	return md != nil && md.FullName() == listValueMessageFullName
}

// valueMessageFullName is the full protobuf name for google.protobuf.Value.
const valueMessageFullName = protoreflect.FullName("google.protobuf.Value")

// isValueWellKnownType returns true when fd points to a google.protobuf.Value
// message field. Value needs special handling: it can hold a Struct, ListValue,
// or a scalar, and the dispatch logic reuses filterStructValue/pruneStructValue.
func isValueWellKnownType(fd protoreflect.FieldDescriptor) bool {
	return fd.Kind() == protoreflect.MessageKind && fd.Message().FullName() == valueMessageFullName
}

// isValueWellKnownMessage returns true when md is the google.protobuf.Value descriptor.
func isValueWellKnownMessage(md protoreflect.MessageDescriptor) bool {
	return md != nil && md.FullName() == valueMessageFullName
}

// filterStruct keeps only keys matching the mask in a google.protobuf.Struct.
func (msk FieldMask) filterStruct(s *structpb.Struct) {
	if s == nil || len(s.GetFields()) == 0 {
		return
	}
	for key, val := range s.GetFields() {
		nested, exists := msk[key]
		if !exists {
			delete(s.Fields, key)
			continue
		}
		if nested == nil || val == nil {
			continue // Keep entire value as-is
		}
		nested.filterStructValue(val)
	}
}

// filterStructValue applies a filter mask to a structpb.Value.
// Struct and ListValue children are recursed into; scalar values are kept as-is.
func (msk FieldMask) filterStructValue(v *structpb.Value) {
	if sv := v.GetStructValue(); sv != nil {
		msk.filterStruct(sv)
	} else if lv := v.GetListValue(); lv != nil {
		msk.filterListValue(lv)
	}
}

// pruneStruct removes keys matching the mask from a google.protobuf.Struct.
func (msk FieldMask) pruneStruct(s *structpb.Struct) {
	if s == nil || len(s.GetFields()) == 0 {
		return
	}
	for key, val := range s.GetFields() {
		nested, exists := msk[key]
		if !exists {
			continue // Not targeted, keep
		}
		if nested == nil {
			delete(s.Fields, key) // Leaf mask, prune
			continue
		}
		if val != nil {
			nested.pruneStructValue(val)
		}
	}
}

// pruneStructValue applies a prune mask to a structpb.Value.
func (msk FieldMask) pruneStructValue(v *structpb.Value) {
	if sv := v.GetStructValue(); sv != nil {
		msk.pruneStruct(sv)
	} else if lv := v.GetListValue(); lv != nil {
		msk.pruneListValue(lv)
	}
}

// buildStructPaths enumerates paths for a google.protobuf.Struct.
func buildStructPaths(s *structpb.Struct, prefix string, paths *[]string) {
	for key, val := range s.GetFields() {
		keyPath := key
		if prefix != "" {
			keyPath = prefix + PathSeparator + key
		}
		*paths = append(*paths, keyPath)
		if val != nil {
			if val.GetStructValue() != nil {
				buildStructPaths(val.GetStructValue(), keyPath, paths)
			} else if val.GetListValue() != nil {
				buildListValuePaths(val.GetListValue(), keyPath, paths)
			}
		}
	}
}

// filterListValue applies the mask to all elements of a ListValue.
// For each Value element that is a Struct, the mask is applied as Struct keys.
// For each Value element that is a ListValue, the mask is applied recursively.
func (msk FieldMask) filterListValue(lv *structpb.ListValue) {
	if lv == nil {
		return
	}
	for _, v := range lv.GetValues() {
		if v == nil {
			continue
		}
		msk.filterStructValue(v)
	}
}

// pruneListValue prunes matching keys from all elements of a ListValue.
func (msk FieldMask) pruneListValue(lv *structpb.ListValue) {
	if lv == nil {
		return
	}
	for _, v := range lv.GetValues() {
		if v == nil {
			continue
		}
		msk.pruneStructValue(v)
	}
}

// buildListValuePaths enumerates paths for a ListValue.
func buildListValuePaths(lv *structpb.ListValue, prefix string, paths *[]string) {
	for i, v := range lv.GetValues() {
		indexPath := prefix + PathSeparator + strconv.Itoa(i)
		*paths = append(*paths, indexPath)
		if v == nil {
			continue
		}
		if sv := v.GetStructValue(); sv != nil {
			buildStructPaths(sv, indexPath, paths)
		} else if nested := v.GetListValue(); nested != nil {
			buildListValuePaths(nested, indexPath, paths)
		}
	}
}

// buildValuePaths enumerates paths for a google.protobuf.Value.
// Struct and ListValue children are recursed into; scalar values produce no sub-paths.
func buildValuePaths(v *structpb.Value, prefix string, paths *[]string) {
	if sv := v.GetStructValue(); sv != nil {
		buildStructPaths(sv, prefix, paths)
	} else if lv := v.GetListValue(); lv != nil {
		buildListValuePaths(lv, prefix, paths)
	}
}

// buildPathsRecursive builds paths for a message. If onlySetFields is true, only set fields are included.
func buildPathsRecursive(msg protoreflect.Message, prefix string, paths *[]string, onlySetFields bool) {
	if !msg.IsValid() {
		return // Changed: return void, not nil slice
	}

	if s, ok := msg.Interface().(*structpb.Struct); ok {
		buildStructPaths(s, prefix, paths)
		return
	}
	if lv, ok := msg.Interface().(*structpb.ListValue); ok {
		buildListValuePaths(lv, prefix, paths)
		return
	}
	if v, ok := msg.Interface().(*structpb.Value); ok {
		buildValuePaths(v, prefix, paths)
		return
	}

	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		// If onlySetFields is true, check if the field is set.
		if onlySetFields && !isFieldSet(msg, fd, v) {
			return true // Skip this field if it's not set and we only want set fields.
		}

		// Field is considered for inclusion (either set or we want all fields).
		path := string(fd.Name())
		if prefix != "" {
			path = prefix + PathSeparator + path
		}
		*paths = append(*paths, path) // Changed: append to *paths

		// Handle nested structures recursively
		switch {
		case fd.Kind() == protoreflect.MessageKind && !fd.IsMap() && !fd.IsList():
			// Nested message
			fieldMsg := v.Message() // Use the actual value
			buildPathsRecursive(fieldMsg, path, paths, onlySetFields)
		case fd.IsMap():
			// Map
			m := v.Map()
			m.Range(func(k protoreflect.MapKey, mapVal protoreflect.Value) bool {
				keyPath := path + PathSeparator + k.String()

				if !onlySetFields {
					*paths = append(*paths, keyPath)
					// If the value is a message, recurse
					if fd.MapValue().Kind() == protoreflect.MessageKind {
						buildPathsRecursive(mapVal.Message(), keyPath, paths, onlySetFields)
					}
					return true
				}

				// If onlySetFields, check if the map value itself is set (for messages)
				// or if the key simply exists (for scalars).

				if fd.MapValue().Kind() == protoreflect.MessageKind {
					if !mapVal.Message().IsValid() {
						// Skip nil message value
						return true
					}
					// Key exists, so include the key path even if message has no set fields.
					*paths = append(*paths, keyPath)
					buildPathsRecursive(mapVal.Message(), keyPath, paths, true)
					return true
				}

				// For scalar values in map:
				// If the key exists, we consider it "set" regardless of the scalar value.
				// Add the path for the key. No further recursion needed for scalars.
				*paths = append(*paths, keyPath)
				return true
			})
		case fd.IsList():
			// List
			lst := v.List()
			for i := range lst.Len() {
				elem := lst.Get(i)
				indexPath := path + PathSeparator + strconv.Itoa(i)

				if !onlySetFields {
					*paths = append(*paths, indexPath)
					// If the element is a message, recurse
					if fd.Kind() == protoreflect.MessageKind {
						buildPathsRecursive(elem.Message(), indexPath, paths, onlySetFields)
					}
					continue
				}

				if fd.Kind() == protoreflect.MessageKind {
					if elem.Message().IsValid() {
						// Element exists, include its index even if message has no set fields.
						*paths = append(*paths, indexPath)
						buildPathsRecursive(elem.Message(), indexPath, paths, true)
					}
					continue // Handled message case (valid or nil)
				}

				// For scalars, check against the zero value of the element's kind
				zeroValue := zeroValueForScalar(fd)
				// Add path only if scalar element is non-zero
				if zeroValue.IsValid() && !elem.Equal(zeroValue) {
					*paths = append(*paths, indexPath)
				}
			}
		}
		return true
	})
}

// validatePath validates a path against a message descriptor.
func validatePath(descriptor protoreflect.MessageDescriptor, path string) error {
	if path == "" {
		return nil
	}

	parts := strings.Split(path, PathSeparator)
	currentDescriptor := descriptor
	var previousField protoreflect.FieldDescriptor

	for i, part := range parts {
		// Check if the previous field was a map or list, in which case the current part is a key/index
		if previousField != nil {
			if previousField.IsMap() {
				// Current part is a map key - any string is valid
				// Check if map value is a message type
				if previousField.MapValue().Kind() == protoreflect.MessageKind {
					if isValueWellKnownMessage(previousField.MapValue().Message()) ||
						isStructWellKnownMessage(previousField.MapValue().Message()) ||
						isListValueWellKnownMessage(previousField.MapValue().Message()) {
						return nil // WKT with dynamic structure: accept any sub-path
					}
					currentDescriptor = previousField.MapValue().Message()
					previousField = nil
					continue
				}
				// Map with scalar values - this should be the last part
				if i < len(parts)-1 {
					return &ValidationError{
						Path:   path,
						Reason: "cannot access nested field in map with scalar values at field: " + parts[i-1],
					}
				}
				return nil
			} else if previousField.IsList() {
				// Current part should be a numeric index or a field in the list element
				if _, err := strconv.Atoi(part); err == nil {
					// This is a list index
					if previousField.Kind() == protoreflect.MessageKind {
						if isValueWellKnownType(previousField) || isStructWellKnownType(previousField) ||
							isListValueWellKnownType(previousField) {
							return nil // WKT with dynamic structure: accept any sub-path
						}
						currentDescriptor = previousField.Message()
						previousField = nil
						continue
					}
					// List of scalars - this should be the last part
					if i < len(parts)-1 {
						return &ValidationError{
							Path:   path,
							Reason: "cannot access field in list of scalar type at field: " + parts[i-1],
						}
					}
					return nil
				}
				// Not a numeric index, treat as field in list element message
				if previousField.Kind() != protoreflect.MessageKind {
					return &ValidationError{
						Path:   path,
						Reason: "cannot access field '" + part + "' in list of scalar type at field: " + parts[i-1],
					}
				}
				currentDescriptor = previousField.Message()
				previousField = nil
				// Continue to validate 'part' as a field name below
			}
		}

		if currentDescriptor == nil {
			return &ValidationError{
				Path:   path,
				Reason: "attempting to access field in non-message type at part: " + part,
			}
		}

		// Find the field descriptor
		field := currentDescriptor.Fields().ByName(protoreflect.Name(part))
		if field == nil {
			return &ValidationError{
				Path:   path,
				Reason: "field '" + part + "' does not exist in message " + string(currentDescriptor.FullName()),
			}
		}

		// Check if we need to go deeper
		if i < len(parts)-1 {
			// Check the field type to determine the next descriptor
			switch {
			case field.IsMap() || field.IsList():
				// Next part will be a map key or list index
				previousField = field
				currentDescriptor = nil // Will be set when processing the key/index
			case field.Kind() == protoreflect.MessageKind:
				if isValueWellKnownType(field) || isStructWellKnownType(field) || isListValueWellKnownType(field) {
					return nil // WKT with dynamic structure: accept any sub-path
				}
				// Nested message
				currentDescriptor = field.Message()
				previousField = nil
			default:
				// Scalar field - can't go deeper
				return &ValidationError{
					Path:   path,
					Reason: "cannot access field '" + parts[i+1] + "' in scalar field: " + part,
				}
			}
		}
	}

	return nil
}

// ValidationError is returned by [FieldMask.Validate] when a field path does not
// match the protobuf message schema. Path is the offending dot-separated path and
// Reason explains what went wrong (missing field, scalar traversal, etc.).
type ValidationError struct {
	Path   string // The invalid path.
	Reason string // Why the path is invalid.
}

// Error returns the error message.
func (e *ValidationError) Error() string {
	return "invalid field mask path '" + e.Path + "': " + e.Reason
}
