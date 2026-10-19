// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"errors"
	"fmt"
	"reflect"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ErrMessageMismatch is returned by [FieldMask.Merge] for messages that are
// nil (or typed-nil), not of one Go type or not of one descriptor.
var ErrMessageMismatch = errors.New("fieldmask: messages are nil or of different types")

// Merge copies the top-level fields msk selects from src to dst, both
// messages of the same type: every selected field is copied from src
// (deep-copied, so dst never shares memory with src), and a selected field
// src does not populate is cleared in dst. Fields outside the mask are kept.
//
// The mask means exactly what it names. An empty mask changes nothing; for
// the fields an update implies when its mask is empty, build the mask from
// src first:
//
//	err := fieldmask.FromSetFields[fieldmask.FieldMask](src, fieldmask.WithSkipFields("id")).Merge(dst, src)
//
// "*" alone ([FieldMask.SelectsAll]) selects every top-level field and "*"
// with other paths is [ErrWildcardNotAlone]. Every other path is resolved
// with [FieldMask.Resolve], so it may use any spelling of its field, and must
// name a whole top-level field: an unknown or nested path is a
// *[ValidationError]. Nil (including typed-nil) messages, or messages not of
// one Go type and one descriptor (a dynamicpb message and the generated type
// of its descriptor do not mix), are [ErrMessageMismatch]. Every check runs
// before the first write, so dst is changed all-or-nothing.
func (msk FieldMask) Merge(dst, src proto.Message) error {
	d, s, err := reflectPair(dst, src)
	if err != nil {
		return err
	}
	fields, err := msk.topLevelFields(d.Descriptor())
	if err != nil {
		return err
	}
	apply(d, s, fields)
	return nil
}

// topLevelFields resolves msk into the whole top-level fields of md it
// selects: every field for "*", otherwise the fields its resolved paths name.
func (msk FieldMask) topLevelFields(md protoreflect.MessageDescriptor) ([]protoreflect.FieldDescriptor, error) {
	all, err := msk.SelectsAll()
	if err != nil {
		return nil, err
	}
	fields := md.Fields()
	if all {
		out := make([]protoreflect.FieldDescriptor, 0, fields.Len())
		for i := range fields.Len() {
			out = append(out, fields.Get(i))
		}
		return out, nil
	}
	resolved, err := msk.Resolve(md)
	if err != nil {
		return nil, err
	}
	paths := resolved.ToPaths()
	out := make([]protoreflect.FieldDescriptor, 0, len(paths))
	for _, path := range paths {
		if len(splitPath(path)) > 1 {
			return nil, &ValidationError{Path: path, Reason: "Merge replaces whole top-level fields; nested paths are not supported"}
		}
		out = append(out, fields.ByName(protoreflect.Name(path)))
	}
	return out, nil
}

// reflectPair returns the reflected dst and src, which must be valid
// (non-nil, not typed-nil) messages of the same Go type and the same
// descriptor. Equal full names are not enough (independently built
// descriptors cannot be mixed), and neither is a shared descriptor: a
// dynamicpb message and the generated type of its descriptor hold nested
// values of different Go types, which the generated setters reject with a
// panic. With one Go type and one descriptor every value of src is valid in
// dst, so the copy cannot fail once the paths are resolved. Converting
// values through a marshal round trip was not chosen: it costs a full
// encode/decode per field and changes unknown-field handling.
func reflectPair(dst, src proto.Message) (protoreflect.Message, protoreflect.Message, error) {
	if dst == nil || src == nil {
		return nil, nil, ErrMessageMismatch
	}
	d, s := dst.ProtoReflect(), src.ProtoReflect()
	if !d.IsValid() || !s.IsValid() {
		return nil, nil, fmt.Errorf("%w: nil message", ErrMessageMismatch)
	}
	if reflect.TypeOf(dst) != reflect.TypeOf(src) || d.Descriptor() != s.Descriptor() {
		return nil, nil, fmt.Errorf("%w: %T (%s) and %T (%s)", ErrMessageMismatch,
			dst, d.Descriptor().FullName(), src, s.Descriptor().FullName())
	}
	return d, s, nil
}

// apply copies fields from a deep copy of s into d, clearing the ones s
// does not populate. Callers resolve every path and check the pair
// ([reflectPair]) first, so nothing here can fail part-way: dst is changed
// all-or-nothing.
func apply(d, s protoreflect.Message, fields []protoreflect.FieldDescriptor) {
	if len(fields) == 0 {
		return
	}
	c := proto.Clone(s.Interface()).ProtoReflect()
	for _, fd := range fields {
		if c.Has(fd) {
			d.Set(fd, c.Get(fd))
		} else {
			d.Clear(fd)
		}
	}
}
