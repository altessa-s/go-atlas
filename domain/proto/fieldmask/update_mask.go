// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"maps"
	"slices"
	"strings"

	"github.com/altessa-s/go-atlas/domain/proto/internal/behavior"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// FieldViolation describes a single field-behavior constraint that was violated
// during [FieldMask.ApplyUpdateMask]. Path is the dot-separated path, Behavior
// is the google.api.field_behavior value that triggered the violation, and
// Reason explains the rejection in human-readable form.
//
// FieldViolation is an alias for [behavior.Violation] so the gRPC interceptor
// can render fieldmask and fieldbehavior violations through the same
// google.rpc.BadRequest FieldViolation mapping.
type FieldViolation = behavior.Violation

// UpdateMaskBehaviorError is returned by [FieldMask.ApplyUpdateMask] when one or
// more google.api.field_behavior annotations are violated (REQUIRED, IMMUTABLE,
// OUTPUT_ONLY, IDENTIFIER). Inspect Violations for per-field details.
//
// Distinct from [fieldbehavior.BehaviorViolationError]: this one carries
// per-field Reason strings produced by the update-mask validator, while the
// fieldbehavior variant aggregates strip-time violations without a reason.
type UpdateMaskBehaviorError struct {
	Violations []FieldViolation // One entry per violated field.
}

func (e *UpdateMaskBehaviorError) Error() string {
	var buf strings.Builder
	buf.WriteString("field behavior violation: ")

	for i, v := range e.Violations {
		if i > 0 {
			buf.WriteString("; ")
		}

		buf.WriteString(v.Path)
		buf.WriteString(": ")
		buf.WriteString(v.Reason)
	}

	return buf.String()
}

// ApplyUpdateMask applies the field mask for update operations:
//  1. Validates field behaviors: REQUIRED, IMMUTABLE, OUTPUT_ONLY, IDENTIFIER (fail-fast)
//  2. Removes OUTPUT_ONLY fields from the mask
//  3. Clears fields NOT in the mask
//  4. Sets default values for fields IN the mask but not populated
//
// An empty mask clears every field, so an explicit empty update_mask updates
// nothing. This differs from an omitted update_mask, which updates all populated
// fields and is handled by the caller before reaching this method.
//
// IDENTIFIER fields (AIP-203) are treated like IMMUTABLE: the identifier names
// the resource and must not be modified by an update. Including such a field
// in the mask produces a violation.
//
// Returns *UpdateMaskBehaviorError if any field behavior constraints are violated.
//
// This ensures the service can distinguish:
//   - "don't update this field" (field not in mask)
//   - "clear this field" (field in mask, set to default)
//   - "update this field" (field in mask, set to value)
//
// Mutates the receiver: IDENTIFIER leaves of msg are added to msk and
// OUTPUT_ONLY entries are removed, so the cleaned msk reflects what was
// actually applied. Callers that want to reuse the original mask should
// pass [FieldMask.Clone] of it.
func (msk FieldMask) ApplyUpdateMask(msg proto.Message) error {
	if msg == nil {
		return nil
	}

	descriptor := msg.ProtoReflect().Descriptor()
	for _, path := range msk.ToPaths() {
		if err := rejectIndexedRepeatedAccess(descriptor, path); err != nil {
			return err
		}
	}

	var violations []FieldViolation
	msk.validateFieldBehaviors(msg, "", &violations)

	if len(violations) > 0 {
		return &UpdateMaskBehaviorError{Violations: violations}
	}

	msk.preserveIdentifierFields(msg)
	msk.removeOutputOnlyFields(msg)
	msk.iterateFilter(msg)
	msk.setDefaultsForUnsetFields(msg)

	return nil
}

// preserveIdentifierFields adds set IDENTIFIER fields (AIP-203) to the mask as
// leaf entries so [FieldMask.iterateFilter] does not clear them. The identifier
// is the resource's routing key — callers normally pass it alongside the
// update_mask without listing it in the mask itself, and dropping it would
// silently break the update path. Only set fields are added: if the caller did
// not provide the identifier, this step is a no-op and downstream logic is
// untouched.
func (msk FieldMask) preserveIdentifierFields(msg proto.Message) {
	prf := msg.ProtoReflect()
	fields := prf.Descriptor().Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		name := string(fd.Name())
		if behavior.Has(fd, annotations.FieldBehavior_IDENTIFIER) {
			if _, exists := msk[name]; !exists && prf.Has(fd) {
				msk[name] = nil
			}
			continue
		}
		// Recurse into nested messages that are already in the mask so
		// nested identifiers within explicitly-updated subtrees are also
		// preserved.
		if fd.Kind() != protoreflect.MessageKind || fd.IsList() || fd.IsMap() || isDynamicWellKnownType(fd) {
			continue
		}
		nested, ok := msk[name]
		if !ok || nested == nil || !prf.Has(fd) {
			continue
		}
		nested.preserveIdentifierFields(prf.Get(fd).Message().Interface())
	}
}

func (msk FieldMask) validateFieldBehaviors(
	msg proto.Message,
	prefix string,
	violations *[]FieldViolation,
) {
	prf := msg.ProtoReflect()
	fields := prf.Descriptor().Fields()

	// Iterate in sorted order so BehaviorViolationError.Violations is deterministic
	// across runs — Violations is rendered into google.rpc.BadRequest by the gRPC
	// interceptor and stable order matters for client diagnostics and tests.
	for _, fieldName := range slices.Sorted(maps.Keys(msk)) {
		nested := msk[fieldName]
		fd := fields.ByName(protoreflect.Name(fieldName))
		if fd == nil {
			continue
		}

		fullPath := behavior.JoinPath(prefix, fieldName)
		isSet := prf.Has(fd)

		switch firstUpdateMaskBehavior(fd) { //nolint:exhaustive // other behaviors have no update-mask semantics and are intentionally treated as optional.
		case annotations.FieldBehavior_REQUIRED:
			if !isSet {
				*violations = append(*violations, FieldViolation{
					Path:     fullPath,
					Behavior: annotations.FieldBehavior_REQUIRED,
					Reason:   "required field cannot be cleared",
				})
				continue
			}

		case annotations.FieldBehavior_IMMUTABLE:
			*violations = append(*violations, FieldViolation{
				Path:     fullPath,
				Behavior: annotations.FieldBehavior_IMMUTABLE,
				Reason:   "immutable field cannot be modified",
			})
			continue

		case annotations.FieldBehavior_IDENTIFIER:
			// IDENTIFIER fields (AIP-203) name the resource and must not be
			// changed by an update — same constraint as IMMUTABLE in this path.
			*violations = append(*violations, FieldViolation{
				Path:     fullPath,
				Behavior: annotations.FieldBehavior_IDENTIFIER,
				Reason:   "identifier field cannot be modified",
			})
			continue

		case annotations.FieldBehavior_OUTPUT_ONLY:
			continue
		}

		if nested != nil && fd.Kind() == protoreflect.MessageKind &&
			!fd.IsList() && !fd.IsMap() && !isDynamicWellKnownType(fd) && isSet {
			nested.validateFieldBehaviors(
				prf.Get(fd).Message().Interface(), fullPath, violations,
			)
		}
	}
}

func (msk FieldMask) removeOutputOnlyFields(msg proto.Message) {
	if msg == nil {
		return
	}
	msk.stripOutputOnlyForDescriptor(msg.ProtoReflect().Descriptor())
}

// stripOutputOnlyForDescriptor removes OUTPUT_ONLY fields at the level
// of desc and recurses into wildcard sub-masks ("*") so OUTPUT_ONLY
// fields under AIP-161 wildcarded subtrees are also stripped from the
// writeback mask. The wildcard key itself is not a real field on desc
// and is skipped at this level; recursion uses the element descriptor
// of the parent repeated / map field.
//
// Per the plan's scope, only "*" sub-masks trigger recursion. The
// historical implicit form (e.g. "aliases.updated_at") is preserved
// as-is so existing callers see the same behavior.
func (msk FieldMask) stripOutputOnlyForDescriptor(desc protoreflect.MessageDescriptor) {
	if desc == nil {
		return
	}
	fields := desc.Fields()
	for fieldName, nested := range msk {
		if fieldName == WildcardSegment {
			continue
		}
		fd := fields.ByName(protoreflect.Name(fieldName))
		if fd == nil {
			continue
		}
		if behavior.Has(fd, annotations.FieldBehavior_OUTPUT_ONLY) {
			delete(msk, fieldName)
			continue
		}
		if nested == nil {
			continue
		}
		wildcardMask, hasWildcard := nested[WildcardSegment]
		if !hasWildcard || wildcardMask == nil {
			continue
		}
		elemDesc := wildcardElementDescriptor(fd)
		if elemDesc == nil {
			continue
		}
		wildcardMask.stripOutputOnlyForDescriptor(elemDesc)
		if len(wildcardMask) == 0 {
			delete(nested, WildcardSegment)
		}
		if len(nested) == 0 {
			delete(msk, fieldName)
		}
	}
}

// wildcardElementDescriptor returns the message descriptor used as the
// recursion target for the "*" wildcard on fd: the list element message
// for repeated fields or the map value message for maps. Returns nil
// for scalar collections and dynamic well-known types whose subtree is
// not navigable by the schema.
func wildcardElementDescriptor(fd protoreflect.FieldDescriptor) protoreflect.MessageDescriptor {
	switch {
	case fd.IsList():
		if fd.Kind() != protoreflect.MessageKind {
			return nil
		}
		m := fd.Message()
		if m == nil || isDynamicWellKnownMessage(m) {
			return nil
		}
		return m
	case fd.IsMap():
		v := fd.MapValue()
		if v == nil || v.Kind() != protoreflect.MessageKind {
			return nil
		}
		m := v.Message()
		if m == nil || isDynamicWellKnownMessage(m) {
			return nil
		}
		return m
	}
	return nil
}

func (msk FieldMask) setDefaultsForUnsetFields(msg proto.Message) {
	prf := msg.ProtoReflect()
	desc := prf.Descriptor()

	for fieldName, nested := range msk {
		fd := desc.Fields().ByName(protoreflect.Name(fieldName))
		if fd == nil {
			continue
		}

		if prf.Has(fd) {
			if nested != nil && fd.Kind() == protoreflect.MessageKind &&
				!fd.IsList() && !fd.IsMap() && !isDynamicWellKnownType(fd) {
				nested.setDefaultsForUnsetFields(prf.Get(fd).Message().Interface())
			}
			continue
		}

		switch {
		case fd.IsList(), fd.IsMap():
			// AIP-134 distinguishes "clear this field" (in mask, absent on the
			// resource) from "leave alone" (not in mask). For maps prf.Mutable
			// already materializes a non-nil empty map; for lists we have to
			// poke the list to allocate a backing array, then truncate so the
			// observable state is "set, length 0" rather than "unset".
			mv := prf.Mutable(fd)
			if fd.IsList() {
				materializeEmptyList(mv.List())
			}
		case fd.Kind() == protoreflect.MessageKind:
			nestedMsg := prf.NewField(fd).Message()
			if nested != nil && !isDynamicWellKnownType(fd) {
				nested.setDefaultsForUnsetFields(nestedMsg.Interface())
			}
			prf.Set(fd, protoreflect.ValueOfMessage(nestedMsg))
		default:
			prf.Set(fd, fd.Default())
		}
	}
}

// materializeEmptyList forces list into a "set, length 0" state. protoreflect
// does not expose a direct "mark as set" primitive for repeated fields; the
// idiomatic workaround is to append a fresh element and immediately truncate.
// Used by setDefaultsForUnsetFields to honor the AIP-134 "clear this field"
// signal from the update mask.
func materializeEmptyList(list protoreflect.List) {
	list.Append(list.NewElement())
	list.Truncate(0)
}

// firstUpdateMaskBehavior collapses the google.api.field_behavior annotation
// list of fd into the single value that governs update-mask semantics:
// REQUIRED, IMMUTABLE, OUTPUT_ONLY, or IDENTIFIER. INPUT_ONLY, UNORDERED_LIST
// and NON_EMPTY_DEFAULT have no update-mask meaning and are reported as
// FIELD_BEHAVIOR_UNSPECIFIED so the caller treats them as optional.
func firstUpdateMaskBehavior(fd protoreflect.FieldDescriptor) annotations.FieldBehavior {
	for _, b := range behavior.Get(fd) {
		switch b { //nolint:exhaustive // INPUT_ONLY, UNORDERED_LIST, NON_EMPTY_DEFAULT have no update-mask semantics.
		case annotations.FieldBehavior_REQUIRED,
			annotations.FieldBehavior_IMMUTABLE,
			annotations.FieldBehavior_OUTPUT_ONLY,
			annotations.FieldBehavior_IDENTIFIER:
			return b
		}
	}

	return annotations.FieldBehavior_FIELD_BEHAVIOR_UNSPECIFIED
}
