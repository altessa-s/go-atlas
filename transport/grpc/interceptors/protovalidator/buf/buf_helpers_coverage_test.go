// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bufhelpers_test

import (
	"strings"
	"testing"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"buf.build/go/protovalidate"
	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	bufhelpers "github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator/buf"
	badrequestv1 "github.com/altessa-s/proto-gen-go/io/altessa/badrequest/v1"
)

// stringMinLen returns field options carrying a buf.validate string.min_len rule.
func stringMinLen(n uint64) *validate.FieldRules {
	return &validate.FieldRules{Type: &validate.FieldRules_String_{String_: &validate.StringRules{MinLen: new(n)}}}
}

func fieldOpts(rules *validate.FieldRules) *descriptorpb.FieldOptions {
	opts := &descriptorpb.FieldOptions{}
	proto.SetExtension(opts, validate.E_Field, rules)
	return opts
}

func mapEntry(name string, keyType descriptorpb.FieldDescriptorProto_Type) *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: new(name),
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: new("key"), Number: new(int32(1)), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: keyType.Enum(), JsonName: new("key")},
			{Name: new("value"), Number: new(int32(2)), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), JsonName: new("value")},
		},
		Options: &descriptorpb.MessageOptions{MapEntry: new(true)},
	}
}

func mapField(name string, number int32, entry string) *descriptorpb.FieldDescriptorProto {
	return &descriptorpb.FieldDescriptorProto{
		Name:     new(name),
		Number:   new(number),
		Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
		Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
		TypeName: new(".covtest.Req." + entry),
		JsonName: new(name),
		Options: fieldOpts(&validate.FieldRules{Type: &validate.FieldRules_Map{Map: &validate.MapRules{
			Values: stringMinLen(2),
		}}}),
	}
}

// requestDescriptor builds, at runtime, a message type annotated with
// protovalidate rules covering scalar, repeated and every map-key kind.
func requestDescriptor(t *testing.T) protoreflect.MessageDescriptor {
	t.Helper()

	fdp := &descriptorpb.FileDescriptorProto{
		Name:       new("covtest/req.proto"),
		Package:    new("covtest"),
		Syntax:     new("proto3"),
		Dependency: []string{"buf/validate/validate.proto"},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Req"),
			Field: []*descriptorpb.FieldDescriptorProto{
				{
					Name: new("name"), Number: new(int32(1)), JsonName: new("name"),
					Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					Options: fieldOpts(stringMinLen(3)),
				},
				{
					Name: new("tags"), Number: new(int32(2)), JsonName: new("tags"),
					Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					Options: fieldOpts(&validate.FieldRules{Type: &validate.FieldRules_Repeated{Repeated: &validate.RepeatedRules{
						Items: stringMinLen(2),
					}}}),
				},
				mapField("by_str", 3, "ByStrEntry"),
				mapField("by_int", 4, "ByIntEntry"),
				mapField("by_uint", 5, "ByUintEntry"),
				mapField("by_bool", 6, "ByBoolEntry"),
			},
			NestedType: []*descriptorpb.DescriptorProto{
				mapEntry("ByStrEntry", descriptorpb.FieldDescriptorProto_TYPE_STRING),
				mapEntry("ByIntEntry", descriptorpb.FieldDescriptorProto_TYPE_INT64),
				mapEntry("ByUintEntry", descriptorpb.FieldDescriptorProto_TYPE_UINT32),
				mapEntry("ByBoolEntry", descriptorpb.FieldDescriptorProto_TYPE_BOOL),
			},
		}},
	}

	fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	require.NoError(t, err)
	return fd.Messages().ByName("Req")
}

func newReq(t *testing.T) *dynamicpb.Message {
	t.Helper()
	return dynamicpb.NewMessage(requestDescriptor(t))
}

func setMap(msg *dynamicpb.Message, field string, key protoreflect.MapKey, value string) {
	fd := msg.Descriptor().Fields().ByName(protoreflect.Name(field))
	msg.Mutable(fd).Map().Set(key, protoreflect.ValueOfString(value))
}

func TestBuildValidator_ValidMessage(t *testing.T) {
	t.Parallel()

	msg := newReq(t)
	msg.Set(msg.Descriptor().Fields().ByName("name"), protoreflect.ValueOfString("alice"))

	validate := bufhelpers.BuildValidator(bufhelpers.BuildValidationFilter())
	require.NoError(t, validate(t.Context(), msg))
}

func TestBuildValidator_ViolationsCarryBadRequestDetails(t *testing.T) {
	t.Parallel()

	msg := newReq(t)
	fields := msg.Descriptor().Fields()
	msg.Set(fields.ByName("name"), protoreflect.ValueOfString("al"))
	tags := msg.Mutable(fields.ByName("tags")).List()
	tags.Append(protoreflect.ValueOfString("ok"))
	tags.Append(protoreflect.ValueOfString("x"))
	setMap(msg, "by_str", protoreflect.ValueOfString("k").MapKey(), "v")
	setMap(msg, "by_int", protoreflect.ValueOfInt64(-7).MapKey(), "v")
	setMap(msg, "by_uint", protoreflect.ValueOfUint32(9).MapKey(), "v")
	setMap(msg, "by_bool", protoreflect.ValueOfBool(true).MapKey(), "v")

	var coderCalls []string
	validate := bufhelpers.BuildValidator(bufhelpers.BuildValidationFilter(),
		bufhelpers.WithReasonCode(func(ruleID, fieldName string) string {
			coderCalls = append(coderCalls, ruleID+"/"+fieldName)
			if fieldName == "name" {
				return "NAME_TOO_SHORT"
			}
			return ""
		}),
	)

	err := validate(t.Context(), msg)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.InvalidArgument, st.Code())
	require.Equal(t, "Validation Failed", st.Message())
	require.True(t, strings.HasPrefix(err.Error(), "validation failed: "), err.Error())
	require.Contains(t, err.Error(), "name: ")
	require.Contains(t, err.Error(), "[string.min_len]")

	var br *badrequestv1.BadRequest
	for _, d := range st.Details() {
		if b, isBR := d.(*badrequestv1.BadRequest); isBR {
			br = b
		}
	}
	require.NotNil(t, br, "status must carry a BadRequest detail")
	require.Len(t, br.GetFieldViolations(), 6)
	require.Len(t, coderCalls, 6, "reason coder is consulted once per violation")

	byPath := make(map[string]*badrequestv1.FieldViolation, len(br.GetFieldViolations()))
	for _, v := range br.GetFieldViolations() {
		byPath[v.GetFieldPath()] = v
	}

	name := byPath["name"]
	require.NotNil(t, name)
	require.Equal(t, "NAME_TOO_SHORT", name.GetCode())
	require.Len(t, name.GetFieldPathComponents(), 1)
	require.Equal(t, "name", name.GetFieldPathComponents()[0].GetName())
	require.Equal(t, badrequestv1.FieldType(descriptorpb.FieldDescriptorProto_TYPE_STRING), name.GetFieldPathComponents()[0].GetType())

	tag := byPath["tags[1]"]
	require.NotNil(t, tag, "paths: %v", keys(byPath))
	require.Nil(t, tag.Code, "empty reason code leaves Code unset")
	comp := tag.GetFieldPathComponents()[0]
	require.True(t, comp.GetRepeatedField())
	require.Equal(t, uint64(1), comp.GetRepeatedIndex())

	str := byPath[`by_str["k"]`]
	require.NotNil(t, str, "paths: %v", keys(byPath))
	require.Equal(t, "k", str.GetFieldPathComponents()[0].GetStringKey())
	require.Equal(t, badrequestv1.FieldType(descriptorpb.FieldDescriptorProto_TYPE_STRING), str.GetFieldPathComponents()[0].GetMapKeyType())
	require.Equal(t, badrequestv1.FieldType(descriptorpb.FieldDescriptorProto_TYPE_STRING), str.GetFieldPathComponents()[0].GetMapValueType())

	intKey := byPath["by_int[-7]"]
	require.NotNil(t, intKey, "paths: %v", keys(byPath))
	require.Equal(t, int64(-7), intKey.GetFieldPathComponents()[0].GetIntKey())

	uintKey := byPath["by_uint[9]"]
	require.NotNil(t, uintKey, "paths: %v", keys(byPath))
	require.Equal(t, uint64(9), uintKey.GetFieldPathComponents()[0].GetUintKey())

	boolKey := byPath["by_bool[true]"]
	require.NotNil(t, boolKey, "paths: %v", keys(byPath))
	require.True(t, boolKey.GetFieldPathComponents()[0].GetBoolKey())
}

type resolverFunc func(ruleID, fieldName string) string

func (f resolverFunc) Resolve(ruleID, fieldName string) string { return f(ruleID, fieldName) }

func TestBuildValidator_WithResolver(t *testing.T) {
	t.Parallel()

	msg := newReq(t)
	msg.Set(msg.Descriptor().Fields().ByName("name"), protoreflect.ValueOfString("a"))

	validate := bufhelpers.BuildValidator(bufhelpers.BuildValidationFilter(),
		bufhelpers.WithResolver(resolverFunc(func(ruleID, _ string) string { return "R_" + ruleID })),
	)
	st, ok := status.FromError(validate(t.Context(), msg))
	require.True(t, ok)

	var br *badrequestv1.BadRequest
	for _, d := range st.Details() {
		if b, isBR := d.(*badrequestv1.BadRequest); isBR {
			br = b
		}
	}
	require.NotNil(t, br)
	require.Len(t, br.GetFieldViolations(), 1)
	require.Equal(t, "R_string.min_len", br.GetFieldViolations()[0].GetCode())
}

func TestBuildValidator_FilterSkipsValidation(t *testing.T) {
	t.Parallel()

	msg := newReq(t)
	msg.Set(msg.Descriptor().Fields().ByName("name"), protoreflect.ValueOfString("a"))

	skipAll := protovalidate.FilterFunc(func(protoreflect.Message, protoreflect.Descriptor) bool { return false })
	require.NoError(t, bufhelpers.BuildValidator(skipAll)(t.Context(), msg))
}

func TestBuildValidationError(t *testing.T) {
	t.Parallel()

	ve := &protovalidate.ValidationError{Violations: []*protovalidate.Violation{
		{Proto: &validate.Violation{
			Field:   &validate.FieldPath{Elements: []*validate.FieldPathElement{{FieldName: new("email")}}},
			RuleId:  new("string.email"),
			Message: new("value must be a valid email address"),
		}},
		{Proto: &validate.Violation{
			RuleId:  new("message.oneof"),
			Message: new("exactly one field is required"),
		}},
	}}

	err := bufhelpers.BuildValidationError(ve)
	require.EqualError(t, err,
		"validation failed: email: value must be a valid email address [string.email]; exactly one field is required [message.oneof]")
}

func keys(m map[string]*badrequestv1.FieldViolation) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
