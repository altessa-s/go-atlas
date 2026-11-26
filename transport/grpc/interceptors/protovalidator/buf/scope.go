// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bufhelpers

import (
	"slices"
	"strings"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"buf.build/go/protovalidate"

	"github.com/altessa-s/go-aip/fieldmask"
	"github.com/altessa-s/go-aip/fieldmask/update"

	"github.com/altessa-s/go-atlas/internal/transport/updatevalidation"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type messageScope struct {
	fields map[protoreflect.FieldNumber]bool
	whole  bool
}

type validationTarget struct {
	message protoreflect.Message
	prefix  []*validate.FieldPathElement
}

// validationScope maps actual message instances to the fields that may run.
type validationScope struct {
	instances   map[proto.Message]messageScope
	targets     []validationTarget
	partialMaps map[string]bool
	selection   *updatevalidation.Selection
}

func buildSelectedScope(resource protoreflect.Message, paths []update.ResolvedPath,
	prefix []*validate.FieldPathElement, selection *updatevalidation.Selection,
) (validationScope, error) {
	names := make([]string, 0, len(paths))
	for _, path := range paths {
		names = append(names, path.Path)
	}
	tree := fieldmask.FromPaths(names...)
	if _, all := tree["*"]; all {
		tree = nil
	}
	scope := validationScope{instances: make(map[proto.Message]messageScope), selection: selection}
	err := scope.walk(resource, tree, prefix, nil)
	return scope, err
}

func (s *validationScope) walk(msg protoreflect.Message, tree fieldmask.FieldMask,
	prefix []*validate.FieldPathElement, route []protoreflect.FieldDescriptor,
) error {
	if !msg.IsValid() {
		return nil
	}
	if _, visited := s.instances[msg.Interface()]; visited {
		return nil
	}
	md := msg.Descriptor()
	covered, err := s.coversMessage(md, tree, route)
	if err != nil {
		return err
	}
	entry := messageScope{whole: covered}
	if tree != nil {
		entry.fields = make(map[protoreflect.FieldNumber]bool)
	}
	s.instances[msg.Interface()] = entry

	fields := md.Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		sub, selected, err := s.fieldScope(tree, fd, len(route) == 0)
		if err != nil {
			return err
		}
		if !selected {
			continue
		}
		fieldRoute := append(slices.Clip(route), fd)
		whole := sub == nil
		if !whole && fd.Message() != nil && !fd.IsMap() {
			whole, err = s.coversMessage(fd.Message(), sub, fieldRoute)
			if err != nil {
				return err
			}
		}
		s.recordPartialMap(fd, sub, prefix)
		if tree != nil && (whole || fd.IsMap()) {
			entry.fields[fd.Number()] = true
		}
		if fd.Message() != nil {
			if err := s.walkField(msg, fd, sub, prefix, whole, fieldRoute); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *validationScope) fieldScope(tree fieldmask.FieldMask, fd protoreflect.FieldDescriptor, root bool) (fieldmask.FieldMask, bool, error) {
	sub, selected := tree[string(fd.Name())]
	if tree == nil || selected || !root {
		return sub, tree == nil || selected, nil
	}
	always, err := s.selection.AlwaysCheck(fd)
	return nil, always, err
}

func (s *validationScope) recordPartialMap(fd protoreflect.FieldDescriptor, sub fieldmask.FieldMask, prefix []*validate.FieldPathElement) {
	if !fd.IsMap() || sub == nil {
		return
	}
	if s.partialMaps == nil {
		s.partialMaps = make(map[string]bool)
	}
	path := validate.FieldPath_builder{Elements: extendPath(prefix, pathElement(fd))}.Build()
	s.partialMaps[protovalidate.FieldPathString(path)] = true
}

func (s *validationScope) walkField(msg protoreflect.Message, fd protoreflect.FieldDescriptor, sub fieldmask.FieldMask,
	prefix []*validate.FieldPathElement, whole bool, route []protoreflect.FieldDescriptor) error {
	switch {
	case fd.IsList():
		list := msg.Get(fd).List()
		for index := range list.Len() {
			part := pathElement(fd)
			part.SetIndex(uint64(index))
			if err := s.walk(list.Get(index).Message(), nil, extendPath(prefix, part), route); err != nil {
				return err
			}
		}
	case fd.IsMap():
		if fd.MapValue().Message() == nil {
			return nil
		}
		for _, key := range sortedMapKeys(msg.Get(fd).Map()) {
			keyTree, ok := updatevalidation.MapKeyScope(sub, key)
			if !ok {
				continue
			}
			child := msg.Get(fd).Map().Get(key).Message()
			childPath := extendPath(prefix, mapPathElement(fd, key))
			if err := s.walkChild(child, keyTree, childPath, true, route); err != nil {
				return err
			}
		}
	case msg.Has(fd):
		childPath := extendPath(prefix, pathElement(fd))
		return s.walkChild(msg.Get(fd).Message(), sub, childPath, whole, route)
	}
	return nil
}

func (s *validationScope) walkChild(msg protoreflect.Message, tree fieldmask.FieldMask, path []*validate.FieldPathElement, traversed bool,
	route []protoreflect.FieldDescriptor) error {
	if !msg.IsValid() {
		return nil
	}
	if !traversed {
		// Buf skips a field's entire subtree when its rules are filtered out.
		// Validate the selected children separately and restore their prefix.
		s.targets = append(s.targets, validationTarget{message: msg, prefix: path})
	}
	return s.walk(msg, tree, path, route)
}

func (s validationScope) allows(msg protoreflect.Message, descriptor protoreflect.Descriptor) bool {
	entry, scoped := s.instances[msg.Interface()]
	if !scoped {
		return false
	}
	switch fd := descriptor.(type) {
	case protoreflect.FieldDescriptor:
		return entry.fields == nil || entry.fields[fd.Number()]
	default:
		return entry.whole
	}
}

func (s *validationScope) coversMessage(md protoreflect.MessageDescriptor, tree fieldmask.FieldMask, route []protoreflect.FieldDescriptor) (bool, error) {
	if tree == nil {
		return true, nil
	}
	if len(tree) == 0 {
		return false, nil
	}
	fields := md.Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		sub, ok := tree[string(fd.Name())]
		fieldRoute := append(slices.Clip(route), fd)
		if !ok {
			if len(route) == 0 {
				always, err := s.selection.AlwaysCheck(fd)
				if err != nil {
					return false, err
				}
				if always {
					continue
				}
			}
			policy, err := s.selection.FieldPolicy(fieldRoute...)
			if err != nil || !policy.OutputOnly {
				return false, err
			}
			continue
		}
		if sub == nil {
			continue
		}
		if fd.IsMap() || fd.IsList() || fd.Message() == nil {
			return false, nil
		}
		covered, err := s.coversMessage(fd.Message(), sub, fieldRoute)
		if err != nil || !covered {
			return false, err
		}
	}
	return true, nil
}

// keepsViolation discards aggregate map rules when only entries were selected.
func (s validationScope) keepsViolation(v *protovalidate.Violation) bool {
	return len(s.partialMaps) == 0 || !s.partialMaps[protovalidate.FieldPathString(v.Proto.GetField())]
}

func sortedMapKeys(values protoreflect.Map) []protoreflect.MapKey {
	keys := make([]protoreflect.MapKey, 0, values.Len())
	values.Range(func(key protoreflect.MapKey, _ protoreflect.Value) bool {
		keys = append(keys, key)
		return true
	})
	slices.SortFunc(keys, func(a, b protoreflect.MapKey) int { return strings.Compare(a.String(), b.String()) })
	return keys
}
