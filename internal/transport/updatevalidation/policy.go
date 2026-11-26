// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package updatevalidation

import (
	"fmt"
	"slices"

	"github.com/altessa-s/go-aip/fieldbehavior"
	"github.com/altessa-s/go-aip/fieldmask/update"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// FieldPolicy inspects a resource field using the registered spec's schema.
func (s *Selection) FieldPolicy(route ...protoreflect.FieldDescriptor) (update.FieldPolicy, error) {
	// ResolveMask requires the resource root to use the spec's exact descriptor.
	// Dynamic message children can use equivalent descriptors from another file.
	md := s.Resource.ProtoReflect().Descriptor()
	schemaRoute := route
	remapped := false
	for i, fd := range route {
		canonical := schemaField(md, fd, i != 0)
		if canonical == nil {
			return update.FieldPolicy{}, fmt.Errorf("field policy at depth %d: %w", i+1, update.ErrMessageMismatch)
		}
		if canonical != fd {
			if !remapped {
				schemaRoute = slices.Clone(route)
				remapped = true
			}
			schemaRoute[i] = canonical
		}
		md = canonical.Message()
		if canonical.IsMap() {
			md = canonical.MapValue().Message()
		}
	}
	return s.Spec.FieldPolicy(schemaRoute...)
}

func schemaField(md protoreflect.MessageDescriptor, fd protoreflect.FieldDescriptor, nested bool) protoreflect.FieldDescriptor {
	if md == nil || fd == nil {
		return nil
	}
	owner := fd.ContainingMessage()
	if fd.IsExtension() {
		if owner == md {
			return fd
		}
		return nil
	}
	canonical := md.Fields().ByNumber(fd.Number())
	if canonical == fd {
		return fd
	}
	if !nested || canonical == nil || owner == nil || owner.FullName() != md.FullName() ||
		canonical.Kind() != fd.Kind() || canonical.Cardinality() != fd.Cardinality() || canonical.IsMap() != fd.IsMap() {
		return nil
	}
	return canonical
}

// AlwaysCheck reports whether a root field is routing input or a supplied
// write-only value, independent of the validation allow-list.
func (s *Selection) AlwaysCheck(fd protoreflect.FieldDescriptor) (bool, error) {
	p, err := s.FieldPolicy(fd)
	return p.Identity || (p.WriteOnly && s.Resource.ProtoReflect().Has(fd)), err
}

// StripOutputOnly removes populated output-only fields using the registered
// policy. Other input remains available for validation and write planning.
func (s *Selection) StripOutputOnly() error {
	return s.strip(s.Resource.ProtoReflect(), nil)
}

func (s *Selection) strip(msg protoreflect.Message, route []protoreflect.FieldDescriptor) error {
	if len(route) > fieldbehavior.DefaultMaxDepth {
		return fieldbehavior.ErrMaxDepthExceeded
	}
	var err error
	msg.Range(func(fd protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		path := append(slices.Clip(route), fd)
		var policy update.FieldPolicy
		policy, err = s.FieldPolicy(path...)
		if err != nil {
			return false
		}
		if policy.OutputOnly {
			msg.Clear(fd)
			return true
		}
		RangeMessages(fd, value, func(child protoreflect.Message) {
			if err == nil {
				err = s.strip(child, path)
			}
		})
		return err == nil
	})
	return err
}
