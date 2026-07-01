// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bufhelpers

import (
	"testing"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"github.com/stretchr/testify/require"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// numericFieldMessage builds a dynamic message "M" with a single field of the
// given proto type and label, and returns the message plus its field descriptor.
func numericFieldMessage(t *testing.T, typ descriptorpb.FieldDescriptorProto_Type, label descriptorpb.FieldDescriptorProto_Label) (protoreflect.Message, protoreflect.FieldDescriptor) {
	t.Helper()
	fdp := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("m.proto"),
		Syntax:  proto.String("proto3"),
		Package: proto.String("bufhelpers.test"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("M"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   proto.String("value"),
				Number: proto.Int32(1),
				Label:  label.Enum(),
				Type:   typ.Enum(),
			}},
		}},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	require.NoError(t, err)
	md := fd.Messages().Get(0)
	return dynamicpb.NewMessage(md), md.Fields().ByNumber(1)
}

// TestLeafNumericValue_RepeatedScalarDoesNotPanic guards against reading a
// repeated numeric field as a scalar, which panics ("cannot convert list to
// int"). Such a leaf must report ok=false instead.
func TestLeafNumericValue_RepeatedScalarDoesNotPanic(t *testing.T) {
	msg, fd := numericFieldMessage(t, descriptorpb.FieldDescriptorProto_TYPE_INT64, descriptorpb.FieldDescriptorProto_LABEL_REPEATED)
	msg.Mutable(fd).List().Append(protoreflect.ValueOfInt64(5))

	path := &validate.FieldPath{Elements: []*validate.FieldPathElement{
		{FieldNumber: proto.Int32(1), FieldName: proto.String("value")},
	}}

	require.NotPanics(t, func() {
		_, _, ok := leafNumericValue(msg, path.GetElements())
		require.False(t, ok, "repeated scalar leaf must not be read as a numeric scalar")
	})
}

// TestLeafNumericValue_SingularScalar reads a plain numeric scalar leaf.
func TestLeafNumericValue_SingularScalar(t *testing.T) {
	msg, fd := numericFieldMessage(t, descriptorpb.FieldDescriptorProto_TYPE_INT64, descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL)
	msg.Set(fd, protoreflect.ValueOfInt64(42))

	path := &validate.FieldPath{Elements: []*validate.FieldPathElement{
		{FieldNumber: proto.Int32(1), FieldName: proto.String("value")},
	}}

	value, _, ok := leafNumericValue(msg, path.GetElements())
	require.True(t, ok)
	require.Equal(t, float64(42), value)
}
