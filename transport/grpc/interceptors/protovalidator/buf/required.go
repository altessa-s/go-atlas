// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bufhelpers

import (
	"slices"
	"strconv"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"buf.build/go/protovalidate"

	"github.com/altessa-s/go-aip/fieldbehavior"
	"github.com/altessa-s/go-aip/fieldmask"
	"github.com/altessa-s/go-aip/fieldmask/update"

	"github.com/altessa-s/go-atlas/internal/transport/updatevalidation"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

const requiredRuleID = "required"

type requiredField struct {
	owner proto.Message
	field protoreflect.FieldNumber
}

type requiredViolations struct {
	blocked  map[requiredField]bool
	seenPath map[string]bool
	errors   []*protovalidate.Violation
}

func (r *requiredViolations) add(msg protoreflect.Message, fd protoreflect.FieldDescriptor, path []*validate.FieldPathElement) {
	if msg != nil && msg.IsValid() {
		r.blocked[requiredField{msg.Interface(), fd.Number()}] = true
	}
	field := validate.FieldPath_builder{Elements: path}.Build()
	name := protovalidate.FieldPathString(field)
	if r.seenPath[name] {
		return
	}
	r.seenPath[name] = true
	r.errors = append(r.errors, &protovalidate.Violation{
		Proto: validate.Violation_builder{
			Field:   field,
			RuleId:  new(requiredRuleID),
			Message: new("value is required"),
		}.Build(),
		FieldDescriptor: fd,
	})
}

func (r *requiredViolations) allows(msg protoreflect.Message, descriptor protoreflect.Descriptor) bool {
	fd, ok := descriptor.(protoreflect.FieldDescriptor)
	return !ok || !r.blocked[requiredField{msg.Interface(), fd.Number()}]
}

// walk checks only the registered resource and its populated descendants.
func (r *requiredViolations) walk(msg protoreflect.Message, prefix []*validate.FieldPathElement,
	scope *validationScope, sel *updatevalidation.Selection, route []protoreflect.FieldDescriptor) error {
	if !msg.IsValid() {
		return nil
	}
	if scope != nil {
		if _, ok := scope.instances[msg.Interface()]; !ok {
			return nil
		}
	}
	fields := msg.Descriptor().Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		path := append(slices.Clip(route), fd)
		missing, err := r.checkField(msg, prefix, fd, scope, sel, path)
		if err != nil {
			return err
		}
		if missing {
			continue
		}
		if fd.Message() != nil && msg.Has(fd) {
			if err := r.walkField(msg, fd, prefix, scope, sel, path); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *requiredViolations) checkField(msg protoreflect.Message, prefix []*validate.FieldPathElement,
	fd protoreflect.FieldDescriptor, scope *validationScope, sel *updatevalidation.Selection, route []protoreflect.FieldDescriptor,
) (bool, error) {
	if (scope != nil && !scope.allows(msg, fd)) || !isEmpty(msg, fd) {
		return false, nil
	}
	policy, err := sel.FieldPolicy(route...)
	if err != nil || !policy.Required {
		return false, err
	}
	fieldPath := extendPath(prefix, pathElement(fd))
	r.add(msg, fd, fieldPath)
	if scope != nil && fd.Message() != nil && !isCollection(fd) && slices.ContainsFunc(route[:len(route)-1], isCollection) {
		return true, r.walkAbsent(fd.Message(), fieldPath, sel, route, make(map[protoreflect.MessageDescriptor]bool))
	}
	return true, nil
}

func isCollection(fd protoreflect.FieldDescriptor) bool {
	return fd.IsList() || fd.IsMap()
}

// walkAbsent checks required descendants of an absent required message in a
// selected collection element. A recursive type cannot supply further values
// once its required message is absent.
func (r *requiredViolations) walkAbsent(md protoreflect.MessageDescriptor, prefix []*validate.FieldPathElement,
	sel *updatevalidation.Selection, route []protoreflect.FieldDescriptor, seen map[protoreflect.MessageDescriptor]bool) error {
	if seen[md] || len(route) > fieldbehavior.DefaultMaxDepth {
		return nil
	}
	seen[md] = true
	defer delete(seen, md)
	fields := md.Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		fieldRoute := append(slices.Clip(route), fd)
		policy, err := sel.FieldPolicy(fieldRoute...)
		if err != nil {
			return err
		}
		if !policy.Required {
			continue
		}
		path := extendPath(prefix, pathElement(fd))
		r.add(nil, fd, path)
		if fd.Message() != nil && !isCollection(fd) {
			if err := r.walkAbsent(fd.Message(), path, sel, fieldRoute, seen); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *requiredViolations) walkField(msg protoreflect.Message, fd protoreflect.FieldDescriptor,
	prefix []*validate.FieldPathElement, scope *validationScope, sel *updatevalidation.Selection,
	route []protoreflect.FieldDescriptor) error {
	switch {
	case fd.IsList():
		list := msg.Get(fd).List()
		for index := range list.Len() {
			part := pathElement(fd)
			part.SetIndex(uint64(index))
			if err := r.walk(list.Get(index).Message(), extendPath(prefix, part), scope, sel, route); err != nil {
				return err
			}
		}
	case fd.IsMap():
		if fd.MapValue().Message() == nil {
			return nil
		}
		values := msg.Get(fd).Map()
		for _, key := range sortedMapKeys(values) {
			if err := r.walk(values.Get(key).Message(), extendPath(prefix, mapPathElement(fd, key)), scope, sel, route); err != nil {
				return err
			}
		}
	default:
		return r.walk(msg.Get(fd).Message(), extendPath(prefix, pathElement(fd)), scope, sel, route)
	}
	return nil
}

func isEmpty(msg protoreflect.Message, fd protoreflect.FieldDescriptor) bool {
	if oneof := fd.ContainingOneof(); oneof != nil && !oneof.IsSynthetic() {
		if active := msg.WhichOneof(oneof); active != nil && active != fd {
			return false
		}
	}
	if !msg.Has(fd) {
		return true
	}
	if fd.IsList() || fd.IsMap() || fd.Message() != nil {
		return false
	}
	value := msg.Get(fd)
	switch fd.Kind() { //nolint:exhaustive // Presence already handles collections and messages.
	case protoreflect.StringKind:
		return value.String() == ""
	case protoreflect.BytesKind:
		return len(value.Bytes()) == 0
	case protoreflect.BoolKind:
		return !value.Bool()
	case protoreflect.EnumKind:
		return value.Enum() == 0
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return value.Int() == 0
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind, protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return value.Uint() == 0
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return value.Float() == 0
	default:
		return false
	}
}

func (r *requiredViolations) addResolvedViolations(msg protoreflect.Message, prefix []*validate.FieldPathElement,
	violations []update.ResolvedViolation, sel *updatevalidation.Selection,
) error {
	for _, violation := range violations {
		if violation.Behavior != annotations.FieldBehavior_REQUIRED {
			continue
		}
		if err := r.addResolved(msg, prefix, violation, sel); err != nil {
			return err
		}
	}
	return nil
}

// addResolved follows declared field names for descriptor lookup, using
// go-aip's structured subscripts to identify the original list and map items.
func (r *requiredViolations) addResolved(msg protoreflect.Message, prefix []*validate.FieldPathElement, violation update.ResolvedViolation,
	sel *updatevalidation.Selection) error {
	tree := fieldmask.FromPaths(violation.Path)
	indexes, keys := violation.ListIndexes, violation.MapKeys
	inCollection := false
	var route []protoreflect.FieldDescriptor
	explicitDepth := 0
	if paths := sel.OriginalMask.GetPaths(); violation.Index >= 0 && violation.Index < len(paths) {
		for raw := fieldmask.FromPaths(paths[violation.Index]); len(raw) > 0; explicitDepth++ {
			_, raw = onlyPath(raw)
		}
	}
	for segment := 0; len(tree) > 0; segment++ {
		name, next := onlyPath(tree)
		fd := msg.Descriptor().Fields().ByName(protoreflect.Name(name))
		if fd == nil {
			return nil
		}
		route = append(route, fd)
		part := pathElement(fd)
		child := msg.Get(fd)
		tree = next
		switch {
		case len(indexes) > 0 && indexes[0].Segment == segment:
			index := indexes[0].Index
			indexes = indexes[1:]
			part.SetIndex(uint64(index))
			inCollection = true
			if index >= child.List().Len() {
				return nil
			}
			msg = child.List().Get(index).Message()
		case len(keys) > 0 && keys[0].Segment == segment:
			key := parseMapKey(fd.MapKey().Kind(), keys[0].Key)
			part = mapPathElement(fd, key)
			inCollection = true
			keys = keys[1:]
			_, tree = onlyPath(tree)
			segment++
			value := child.Map().Get(key)
			if !value.IsValid() || fd.MapValue().Message() == nil {
				return nil
			}
			msg = value.Message()
		default:
			prefix = extendPath(prefix, part)
			if len(tree) == 0 {
				r.add(msg, fd, prefix)
				return nil
			}
			// Whole map entries can expand into leaves below an absent optional
			// message. Match list-element validation unless the mask names a
			// descendant explicitly, which retains clear intent.
			if inCollection && explicitDepth <= segment+1 && !msg.Has(fd) {
				policy, err := sel.FieldPolicy(route...)
				if err != nil || !policy.Required {
					return err
				}
			}
			msg = child.Message()
			continue
		}
		prefix = extendPath(prefix, part)
	}
	return nil
}

func onlyPath(tree fieldmask.FieldMask) (string, fieldmask.FieldMask) {
	for name, next := range tree {
		return name, next
	}
	return "", nil
}

func parseMapKey(kind protoreflect.Kind, key string) protoreflect.MapKey {
	switch kind { //nolint:exhaustive // Protobuf map keys are bool, string or integer.
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(key == "true").MapKey()
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		value, _ := strconv.ParseInt(key, 10, 32) //nolint:errcheck // go-aip supplies a valid key
		return protoreflect.ValueOfInt32(int32(value)).MapKey()
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		value, _ := strconv.ParseInt(key, 10, 64) //nolint:errcheck // go-aip supplies a valid key
		return protoreflect.ValueOfInt64(value).MapKey()
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		value, _ := strconv.ParseUint(key, 10, 32) //nolint:errcheck // go-aip supplies a valid key
		return protoreflect.ValueOfUint32(uint32(value)).MapKey()
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		value, _ := strconv.ParseUint(key, 10, 64) //nolint:errcheck // go-aip supplies a valid key
		return protoreflect.ValueOfUint64(value).MapKey()
	default:
		return protoreflect.ValueOfString(key).MapKey()
	}
}

func extendPath(path []*validate.FieldPathElement, element *validate.FieldPathElement) []*validate.FieldPathElement {
	return append(slices.Clip(path), element)
}

func pathElement(fd protoreflect.FieldDescriptor) *validate.FieldPathElement {
	name := string(fd.Name())
	return validate.FieldPathElement_builder{
		FieldNumber: new(int32(fd.Number())),
		FieldName:   new(name),
		FieldType:   descriptorpb.FieldDescriptorProto_Type(fd.Kind()).Enum(),
	}.Build()
}

func mapPathElement(fd protoreflect.FieldDescriptor, key protoreflect.MapKey) *validate.FieldPathElement {
	part := pathElement(fd)
	part.SetKeyType(descriptorpb.FieldDescriptorProto_Type(fd.MapKey().Kind()))
	part.SetValueType(descriptorpb.FieldDescriptorProto_Type(fd.MapValue().Kind()))
	switch fd.MapKey().Kind() { //nolint:exhaustive // Protobuf map keys are bool, string or integer.
	case protoreflect.BoolKind:
		part.SetBoolKey(key.Bool())
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		part.SetIntKey(key.Int())
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind, protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		part.SetUintKey(key.Uint())
	default:
		part.SetStringKey(key.String())
	}
	return part
}
