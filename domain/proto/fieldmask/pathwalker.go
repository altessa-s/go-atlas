// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"strconv"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// pathWalker walks a dot-separated field path against a message descriptor,
// invoking per-segment callbacks that customize traversal semantics.
//
// The walker centralizes the schema-traversal state machine shared by
// [validatePath] (strict schema validation — every malformed segment is an
// error) and [rejectIndexedRepeatedAccess] (single-rule check for numeric
// indexes on repeated fields — all unrelated path issues silently ignored).
//
// All callbacks are optional. A nil callback is interpreted as "no-op, keep
// walking when possible". A callback that returns a non-nil error stops
// traversal and surfaces the error. The walker honors AIP-161 wildcard
// semantics and short-circuits into the dynamic well-known types
// (google.protobuf.Struct/ListValue/Value), whose subtree is opaque to the
// static schema.
type pathWalker struct {
	// onListIndex fires for a numeric segment that follows a list field.
	// Both message-element and scalar-element lists invoke the callback.
	// validatePath leaves this nil; rejectIndexedRepeatedAccess uses it as
	// its single rule.
	onListIndex func(list protoreflect.FieldDescriptor, segment string, isLast bool) error

	// onListWildcardOnScalar fires when "*" follows a scalar list and the
	// wildcard is not the terminal segment. Used by validatePath to reject
	// "tags.*.field" when tags is a list of strings.
	onListWildcardOnScalar func(list protoreflect.FieldDescriptor) error

	// onListIndexOnScalar fires when a numeric index follows a scalar list
	// and the index is not the terminal segment.
	onListIndexOnScalar func(list protoreflect.FieldDescriptor) error

	// onListFieldOnScalar fires when a non-numeric, non-wildcard segment
	// follows a scalar list (the caller meant to address a field on the
	// list element, but the element has no fields).
	onListFieldOnScalar func(list protoreflect.FieldDescriptor, segment string) error

	// onMapScalarValueDeref fires when the path continues beyond a map-key
	// segment on a map whose values are scalars.
	onMapScalarValueDeref func(mapField protoreflect.FieldDescriptor) error

	// onMissingField fires when a field-name lookup returns nil on the
	// current message descriptor.
	onMissingField func(parent protoreflect.MessageDescriptor, segment string) error

	// onNonMessageContext fires when more segments remain but the current
	// descriptor is nil (typically because the previous list/map step
	// ended up in a scalar or WKT context).
	onNonMessageContext func(segment string) error

	// onScalarDeref fires when a non-terminal segment tries to descend
	// through a scalar field.
	onScalarDeref func(scalar protoreflect.FieldDescriptor, next string) error
}

// Walk traverses path against root, invoking the configured callbacks at
// each significant transition. Returns nil when path is empty or fully
// consumed without any callback signaling an error.
func (w pathWalker) Walk(root protoreflect.MessageDescriptor, path string) error {
	if path == "" {
		return nil
	}

	parts := splitPath(path)
	currentDescriptor := root
	var previousField protoreflect.FieldDescriptor

	for i, part := range parts {
		isLast := i == len(parts)-1

		if previousField != nil {
			switch {
			case previousField.IsList():
				next, consumed, stop, err := w.afterList(previousField, part, isLast)
				if err != nil {
					return err
				}
				if stop {
					return nil
				}
				currentDescriptor = next
				previousField = nil
				if consumed {
					continue
				}
				// Fall through: 'part' is a field name on the list-element
				// descriptor (currentDescriptor) and must still be looked up.

			case previousField.IsMap():
				next, stop, err := w.afterMap(previousField, isLast)
				if err != nil {
					return err
				}
				if stop {
					return nil
				}
				currentDescriptor = next
				previousField = nil
				continue
			}
		}

		if currentDescriptor == nil {
			if w.onNonMessageContext != nil {
				if err := w.onNonMessageContext(part); err != nil {
					return err
				}
			}
			return nil
		}

		field := currentDescriptor.Fields().ByName(protoreflect.Name(part))
		if field == nil {
			if w.onMissingField != nil {
				if err := w.onMissingField(currentDescriptor, part); err != nil {
					return err
				}
			}
			return nil
		}

		if isLast {
			return nil
		}

		switch {
		case field.IsList() || field.IsMap():
			previousField = field
			currentDescriptor = nil
		case field.Kind() == protoreflect.MessageKind:
			if isDynamicWellKnownType(field) {
				return nil
			}
			currentDescriptor = field.Message()
			previousField = nil
		default:
			if w.onScalarDeref != nil {
				if err := w.onScalarDeref(field, parts[i+1]); err != nil {
					return err
				}
			}
			return nil
		}
	}

	return nil
}

// afterList handles a segment that immediately follows a list field. Returns
// (next, consumed, stop, err):
//   - stop=true: walker returns nil immediately (e.g. WKT short-circuit, or
//     a scalar-list case the caller chose to terminate silently).
//   - consumed=true: segment is fully consumed (wildcard or numeric index);
//     walker advances to the next iteration with next as currentDescriptor.
//   - consumed=false: segment is a field-name on the list-element message;
//     walker falls through to look it up on next.
func (w pathWalker) afterList(list protoreflect.FieldDescriptor, part string, isLast bool) (
	next protoreflect.MessageDescriptor,
	consumed bool,
	stop bool,
	err error,
) {
	isMessageList := list.Kind() == protoreflect.MessageKind
	elemMsg := list.Message()
	isWKT := isMessageList && elemMsg != nil && isDynamicWellKnownMessage(elemMsg)

	switch {
	case part == WildcardSegment:
		if isMessageList {
			if isWKT {
				return nil, true, true, nil
			}
			return elemMsg, true, false, nil
		}
		if !isLast && w.onListWildcardOnScalar != nil {
			return nil, true, false, w.onListWildcardOnScalar(list)
		}
		return nil, true, true, nil

	case isNumericSegment(part):
		if w.onListIndex != nil {
			if cbErr := w.onListIndex(list, part, isLast); cbErr != nil {
				return nil, true, false, cbErr
			}
		}
		if isMessageList {
			if isWKT {
				return nil, true, true, nil
			}
			return elemMsg, true, false, nil
		}
		if !isLast && w.onListIndexOnScalar != nil {
			return nil, true, false, w.onListIndexOnScalar(list)
		}
		return nil, true, true, nil

	default:
		// Non-numeric, non-wildcard: a field-name on the list-element message.
		if !isMessageList {
			if w.onListFieldOnScalar != nil {
				return nil, true, false, w.onListFieldOnScalar(list, part)
			}
			return nil, true, true, nil
		}
		if isWKT {
			return nil, true, true, nil
		}
		return elemMsg, false, false, nil
	}
}

// afterMap handles the segment that immediately follows a map field (a map
// key). Returns (next, stop, err). Map keys are always fully consumed —
// callers do not fall through to look them up as fields.
func (w pathWalker) afterMap(mapField protoreflect.FieldDescriptor, isLast bool) (
	next protoreflect.MessageDescriptor,
	stop bool,
	err error,
) {
	mv := mapField.MapValue()
	if mv != nil && mv.Kind() == protoreflect.MessageKind {
		valMsg := mv.Message()
		if isDynamicWellKnownMessage(valMsg) {
			return nil, true, nil
		}
		return valMsg, false, nil
	}

	// Map with scalar values — path continuation is meaningless.
	if !isLast && w.onMapScalarValueDeref != nil {
		return nil, true, w.onMapScalarValueDeref(mapField)
	}
	return nil, true, nil
}

// isNumericSegment reports whether s is a non-empty base-10 integer literal,
// the textual form of a list index in a dot-separated field path.
func isNumericSegment(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
