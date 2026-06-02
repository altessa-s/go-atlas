// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"strings"

	"github.com/altessa-s/go-atlas/domain/proto/internal/behavior"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// FieldViolation describes a single field-behavior constraint that was violated
// during [FieldMask.ApplyUpdateMask]. Field is the dot-separated path, and
// Description explains the rejection reason.
type FieldViolation struct {
	Field       string // Dot-separated path of the offending field.
	Description string // Human-readable reason for the violation.
}

// BehaviorViolationError is returned by [FieldMask.ApplyUpdateMask] when one or
// more google.api.field_behavior annotations are violated (REQUIRED, IMMUTABLE,
// OUTPUT_ONLY). Inspect Violations for per-field details.
type BehaviorViolationError struct {
	Violations []FieldViolation // One entry per violated field.
}

func (e *BehaviorViolationError) Error() string {
	buf := strings.Builder{}
	buf.WriteString("field behavior violation: ")

	for i, v := range e.Violations {
		if i > 0 {
			buf.WriteString("; ")
		}

		buf.WriteString(v.Field)
		buf.WriteString(": ")
		buf.WriteString(v.Description)
	}

	return buf.String()
}

type fieldBehavior int

const (
	fieldBehaviorOptional fieldBehavior = iota
	fieldBehaviorRequired
	fieldBehaviorImmutable
	fieldBehaviorOutputOnly
	fieldBehaviorIdentifier
)

// ApplyUpdateMask applies the field mask for update operations:
//  1. Validates field behaviors: REQUIRED, IMMUTABLE, OUTPUT_ONLY, IDENTIFIER (fail-fast)
//  2. Removes OUTPUT_ONLY fields from the mask
//  3. Clears fields NOT in the mask
//  4. Sets default values for fields IN the mask but not populated
//
// IDENTIFIER fields (AIP-203) are treated like IMMUTABLE: the identifier names
// the resource and must not be modified by an update. Including such a field
// in the mask produces a violation.
//
// Returns *BehaviorViolationError if any field behavior constraints are violated.
//
// This ensures the service can distinguish:
//   - "don't update this field" (field not in mask)
//   - "clear this field" (field in mask, set to default)
//   - "update this field" (field in mask, set to value)
func (msk FieldMask) ApplyUpdateMask(msg proto.Message) error {
	if len(msk) == 0 {
		return nil
	}

	if msg != nil {
		descriptor := msg.ProtoReflect().Descriptor()
		for _, path := range msk.ToPaths() {
			if err := rejectIndexedRepeatedAccess(descriptor, path); err != nil {
				return err
			}
		}
	}

	var violations []FieldViolation
	msk.validateFieldBehaviors(msg, "", &violations)

	if len(violations) > 0 {
		return &BehaviorViolationError{Violations: violations}
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
		if getFieldBehavior(fd) == fieldBehaviorIdentifier {
			if _, exists := msk[name]; !exists && prf.Has(fd) {
				msk[name] = nil
			}
			continue
		}
		// Recurse into nested messages that are already in the mask so
		// nested identifiers within explicitly-updated subtrees are also
		// preserved.
		if fd.Kind() != protoreflect.MessageKind || fd.IsList() || fd.IsMap() ||
			isValueWellKnownType(fd) || isStructWellKnownType(fd) || isListValueWellKnownType(fd) {
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

	for fieldName, nested := range msk {
		fd := fields.ByName(protoreflect.Name(fieldName))
		if fd == nil {
			continue
		}

		fullPath := fieldName
		if prefix != "" {
			fullPath = prefix + PathSeparator + fieldName
		}

		isSet := prf.Has(fd)
		behavior := getFieldBehavior(fd)

		switch behavior {
		case fieldBehaviorOptional:
			// No validation needed for optional fields.
		case fieldBehaviorRequired:
			if !isSet {
				*violations = append(*violations, FieldViolation{
					Field:       fullPath,
					Description: "required field cannot be cleared",
				})
				continue
			}

		case fieldBehaviorImmutable:
			*violations = append(*violations, FieldViolation{
				Field:       fullPath,
				Description: "immutable field cannot be modified",
			})
			continue

		case fieldBehaviorIdentifier:
			// IDENTIFIER fields (AIP-203) name the resource and must not be
			// changed by an update — same constraint as IMMUTABLE in this path.
			*violations = append(*violations, FieldViolation{
				Field:       fullPath,
				Description: "identifier field cannot be modified",
			})
			continue

		case fieldBehaviorOutputOnly:
			continue
		}

		if nested != nil && fd.Kind() == protoreflect.MessageKind &&
			!fd.IsList() && !fd.IsMap() && !isValueWellKnownType(fd) && !isStructWellKnownType(fd) &&
			!isListValueWellKnownType(fd) && isSet {
			nested.validateFieldBehaviors(
				prf.Get(fd).Message().Interface(), fullPath, violations,
			)
		}
	}
}

func (msk FieldMask) removeOutputOnlyFields(msg proto.Message) {
	fields := msg.ProtoReflect().Descriptor().Fields()
	for fieldName := range msk {
		fd := fields.ByName(protoreflect.Name(fieldName))
		if fd != nil && getFieldBehavior(fd) == fieldBehaviorOutputOnly {
			delete(msk, fieldName)
		}
	}
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
				!fd.IsList() && !fd.IsMap() && !isValueWellKnownType(fd) && !isStructWellKnownType(fd) &&
				!isListValueWellKnownType(fd) {
				nested.setDefaultsForUnsetFields(prf.Get(fd).Message().Interface())
			}
			continue
		}

		switch {
		case fd.IsList(), fd.IsMap():
			// Force non-nil empty to distinguish clear from absent.
			mv := prf.Mutable(fd)
			if fd.IsList() {
				list := mv.List()
				list.Append(list.NewElement())
				list.Truncate(0)
			}
		case fd.Kind() == protoreflect.MessageKind:
			nestedMsg := prf.NewField(fd).Message()
			if nested != nil && !isValueWellKnownType(fd) && !isStructWellKnownType(fd) &&
				!isListValueWellKnownType(fd) {
				nested.setDefaultsForUnsetFields(nestedMsg.Interface())
			}
			prf.Set(fd, protoreflect.ValueOfMessage(nestedMsg))
		default:
			prf.Set(fd, fd.Default())
		}
	}
}

// getFieldBehavior collapses the google.api.field_behavior annotation list of
// fd into the single fieldBehavior value that governs update-mask semantics.
// INPUT_ONLY, UNORDERED_LIST and NON_EMPTY_DEFAULT have no update-mask meaning
// and are reported as fieldBehaviorOptional.
func getFieldBehavior(fd protoreflect.FieldDescriptor) fieldBehavior {
	for _, b := range behavior.Get(fd) {
		switch b { //nolint:exhaustive // INPUT_ONLY, UNORDERED_LIST, NON_EMPTY_DEFAULT have no update-mask semantics.
		case annotations.FieldBehavior_REQUIRED:
			return fieldBehaviorRequired
		case annotations.FieldBehavior_IMMUTABLE:
			return fieldBehaviorImmutable
		case annotations.FieldBehavior_OUTPUT_ONLY:
			return fieldBehaviorOutputOnly
		case annotations.FieldBehavior_IDENTIFIER:
			return fieldBehaviorIdentifier
		}
	}

	return fieldBehaviorOptional
}
