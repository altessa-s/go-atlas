// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"strings"

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
)

// ApplyUpdateMask applies the field mask for update operations:
//  1. Validates field behaviors: REQUIRED, IMMUTABLE, OUTPUT_ONLY (fail-fast)
//  2. Removes OUTPUT_ONLY fields from the mask
//  3. Clears fields NOT in the mask
//  4. Sets default values for fields IN the mask but not populated
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

	var violations []FieldViolation
	msk.validateFieldBehaviors(msg, "", &violations)

	if len(violations) > 0 {
		return &BehaviorViolationError{Violations: violations}
	}

	msk.removeOutputOnlyFields(msg)
	msk.iterateFilter(msg)
	msk.setDefaultsForUnsetFields(msg)

	return nil
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

// getFieldBehavior reads the google.api.field_behavior annotation from a field descriptor.
func getFieldBehavior(fd protoreflect.FieldDescriptor) fieldBehavior {
	opts := fd.Options()
	if opts == nil {
		return fieldBehaviorOptional
	}

	behaviors, ok := proto.GetExtension(opts, annotations.E_FieldBehavior).([]annotations.FieldBehavior)
	if !ok {
		return fieldBehaviorOptional
	}

	for _, b := range behaviors {
		switch b { //nolint:exhaustive // Only REQUIRED, IMMUTABLE, OUTPUT_ONLY are relevant for update mask semantics.
		case annotations.FieldBehavior_REQUIRED:
			return fieldBehaviorRequired
		case annotations.FieldBehavior_IMMUTABLE:
			return fieldBehaviorImmutable
		case annotations.FieldBehavior_OUTPUT_ONLY:
			return fieldBehaviorOutputOnly
		}
	}

	return fieldBehaviorOptional
}
