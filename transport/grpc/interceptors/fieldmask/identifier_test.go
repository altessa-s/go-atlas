// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/fieldmask/update"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pbfieldmask "github.com/altessa-s/go-aip/fieldmask"
	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// TestServerInterceptor_UpdateMask_ParentOfIdentifierExpanded pins that an
// explicit update_mask naming a message whole that holds a server-managed
// field (an IDENTIFIER, an OUTPUT_ONLY), populated or absent, is expanded to
// the message's other fields: the handler never gets a mask authorizing the
// replacement (or clearing) of the parent together with its identifier.
// Naming the parent's other fields passes and keeps the nested identifier out
// of the handler's mask.
func TestServerInterceptor_UpdateMask_ParentOfIdentifierExpanded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resource *pb.Resource
	}{
		{name: "populated parent", resource: &pb.Resource{Id: "id-1", Profile: &pb.Profile{Id: "replacement", DisplayName: "Ann"}}},
		{name: "absent parent", resource: &pb.Resource{Id: "id-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uni := newUnary(t)
			req := &pb.UpdateResourceRequest{
				Resource:   tt.resource,
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"profile"}},
			}
			seenReq, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
			require.NoError(t, err)
			got, ok := seenReq.(*pb.UpdateResourceRequest)
			require.True(t, ok)
			paths := got.GetUpdateMask().GetPaths()
			slices.Sort(paths)
			require.Equal(t, []string{"profile.display_name", "profile.secret"}, paths)
		})
	}

	uni := newUnary(t)
	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Id: "id-1", Profile: &pb.Profile{Id: "p-1", DisplayName: "Ann"}},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"profile.display_name"}},
	}
	seenReq, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)
	got, ok := seenReq.(*pb.UpdateResourceRequest)
	require.True(t, ok)
	require.Equal(t, "p-1", got.GetResource().GetProfile().GetId())
	paths := got.GetUpdateMask().GetPaths()
	slices.Sort(paths)
	require.Equal(t, []string{"profile.display_name"}, paths)
}

// grandchildDescriptor builds an update request whose resource has a
// populated parent with an absent protected grandchild:
//
//	message Child   { string id = 1 [IDENTIFIER]; }
//	message Frozen  { string code = 1 [IMMUTABLE]; }
//	message Profile { string name = 1; Child child = 2; Frozen frozen = 3; Profile next = 4; }
//	message Res     { string name = 1; Profile profile = 2; }
//	message UpdateResRequest { Res resource = 1; google.protobuf.FieldMask update_mask = 2; }
func grandchildDescriptor(tb testing.TB) protoreflect.MessageDescriptor {
	tb.Helper()

	behaviorOpts := func(b annotations.FieldBehavior) *descriptorpb.FieldOptions {
		o := &descriptorpb.FieldOptions{}
		proto.SetExtension(o, annotations.E_FieldBehavior, []annotations.FieldBehavior{b})
		return o
	}
	msg := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	field := func(name string, num int32, typ descriptorpb.FieldDescriptorProto_Type, typeName string, opts *descriptorpb.FieldOptions) *descriptorpb.FieldDescriptorProto {
		f := &descriptorpb.FieldDescriptorProto{Name: new(name), Number: new(num), Type: &typ, Label: &opt, Options: opts}
		if typeName != "" {
			f.TypeName = new(typeName)
		}
		return f
	}

	fdp := &descriptorpb.FileDescriptorProto{
		Name:       new("fieldmask_grandchild_test.proto"),
		Package:    new("fieldmaskgctest"),
		Syntax:     new("proto3"),
		Dependency: []string{"google/protobuf/field_mask.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: new("Child"), Field: []*descriptorpb.FieldDescriptorProto{
				field("id", 1, str, "", behaviorOpts(annotations.FieldBehavior_IDENTIFIER)),
			}},
			{Name: new("Frozen"), Field: []*descriptorpb.FieldDescriptorProto{
				field("code", 1, str, "", behaviorOpts(annotations.FieldBehavior_IMMUTABLE)),
			}},
			{Name: new("Profile"), Field: []*descriptorpb.FieldDescriptorProto{
				field("name", 1, str, "", nil),
				field("child", 2, msg, ".fieldmaskgctest.Child", nil),
				field("frozen", 3, msg, ".fieldmaskgctest.Frozen", nil),
				field("next", 4, msg, ".fieldmaskgctest.Profile", nil),
			}},
			{Name: new("Res"), Field: []*descriptorpb.FieldDescriptorProto{
				field("name", 1, str, "", nil),
				field("profile", 2, msg, ".fieldmaskgctest.Profile", nil),
			}},
			{Name: new("UpdateResRequest"), Field: []*descriptorpb.FieldDescriptorProto{
				field("resource", 1, msg, ".fieldmaskgctest.Res", nil),
				field("update_mask", 2, msg, ".google.protobuf.FieldMask", nil),
			}},
		},
	}
	fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	require.NoError(tb, err)
	return fd.Messages().ByName("UpdateResRequest")
}

// TestServerInterceptor_EmptyUpdateMask_AbsentProtectedGrandchild verifies the compatibility option is a no-op.
// An explicit empty mask stays empty with nested protected fields.
func TestServerInterceptor_EmptyUpdateMask_AbsentProtectedGrandchild(t *testing.T) {
	t.Parallel()

	reqMD := grandchildDescriptor(t)
	resFD := reqMD.Fields().ByName("resource")
	resMD := resFD.Message()
	profFD := resMD.Fields().ByName("profile")
	profMD := profFD.Message()

	profile := dynamicpb.NewMessage(profMD)
	profile.Set(profMD.Fields().ByName("name"), protoreflect.ValueOfString("Ann"))
	res := dynamicpb.NewMessage(resMD)
	res.Set(profFD, protoreflect.ValueOfMessage(profile))
	req := dynamicpb.NewMessage(reqMD)
	req.Set(resFD, protoreflect.ValueOfMessage(res))
	req.Set(reqMD.Fields().ByName("update_mask"), protoreflect.ValueOfMessage((&fieldmaskpb.FieldMask{}).ProtoReflect()))

	uni := newUnary(t)
	seenReq, _, err := runUnary(t, uni, "/x.v1.X/UpdateRes", req, dynamicpb.NewMessage(resMD), nil)
	require.NoError(t, err, "an editable leaf under a parent with absent protected descendants must pass")

	got, ok := seenReq.(*dynamicpb.Message)
	require.True(t, ok)
	mask, ok := got.Get(reqMD.Fields().ByName("update_mask")).Message().Interface().(*fieldmaskpb.FieldMask)
	require.True(t, ok)
	require.Empty(t, mask.GetPaths())
}

// outputOnlyDescriptor builds an update request whose resource has a nested
// field that is both IMMUTABLE and OUTPUT_ONLY:
//
//	message Sub { string code = 1 [IMMUTABLE, OUTPUT_ONLY]; string note = 2; string ref = 3 [IDENTIFIER, OUTPUT_ONLY]; }
//	message Extra { string when = 1 [OUTPUT_ONLY]; }
//	message Box { string label = 1; Extra extra = 2; }
//	message Res { Sub sub = 1; string title = 2; string rid = 3 [OUTPUT_ONLY, IDENTIFIER]; Box box = 4; }
//	message UpdateResRequest { Res resource = 1; google.protobuf.FieldMask update_mask = 2; }
func outputOnlyDescriptor(tb testing.TB) protoreflect.MessageDescriptor {
	tb.Helper()

	behaviorOpts := func(b ...annotations.FieldBehavior) *descriptorpb.FieldOptions {
		o := &descriptorpb.FieldOptions{}
		proto.SetExtension(o, annotations.E_FieldBehavior, b)
		return o
	}
	codeOpts := behaviorOpts(annotations.FieldBehavior_IMMUTABLE, annotations.FieldBehavior_OUTPUT_ONLY)
	msg := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL

	fdp := &descriptorpb.FileDescriptorProto{
		Name:       new("fieldmask_outputonly_test.proto"),
		Package:    new("fieldmaskootest"),
		Syntax:     new("proto3"),
		Dependency: []string{"google/protobuf/field_mask.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: new("Sub"), Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("code"), Number: proto.Int32(1), Type: &str, Label: &opt, Options: codeOpts},
				{Name: new("note"), Number: proto.Int32(2), Type: &str, Label: &opt},
				{Name: new("ref"), Number: proto.Int32(3), Type: &str, Label: &opt,
					Options: behaviorOpts(annotations.FieldBehavior_IDENTIFIER, annotations.FieldBehavior_OUTPUT_ONLY)},
			}},
			{Name: new("Extra"), Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("when"), Number: proto.Int32(1), Type: &str, Label: &opt,
					Options: behaviorOpts(annotations.FieldBehavior_OUTPUT_ONLY)},
			}},
			{Name: new("Box"), Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("label"), Number: proto.Int32(1), Type: &str, Label: &opt},
				{Name: new("extra"), Number: proto.Int32(2), Type: &msg, Label: &opt, TypeName: new(".fieldmaskootest.Extra")},
			}},
			{Name: new("Res"), Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("sub"), Number: proto.Int32(1), Type: &msg, Label: &opt, TypeName: new(".fieldmaskootest.Sub")},
				{Name: new("title"), Number: proto.Int32(2), Type: &str, Label: &opt},
				{Name: new("rid"), Number: proto.Int32(3), Type: &str, Label: &opt,
					Options: behaviorOpts(annotations.FieldBehavior_OUTPUT_ONLY, annotations.FieldBehavior_IDENTIFIER)},
				{Name: new("box"), Number: proto.Int32(4), Type: &msg, Label: &opt, TypeName: new(".fieldmaskootest.Box")},
			}},
			{Name: new("UpdateResRequest"), Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("resource"), Number: proto.Int32(1), Type: &msg, Label: &opt, TypeName: new(".fieldmaskootest.Res")},
				{Name: new("update_mask"), Number: proto.Int32(2), Type: &msg, Label: &opt, TypeName: new(".google.protobuf.FieldMask")},
			}},
		},
	}
	fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	require.NoError(tb, err)
	return fd.Messages().ByName("UpdateResRequest")
}

// TestServerInterceptor_UpdateMask_NestedOutputOnlyDropped pins that a nested
// OUTPUT_ONLY entry — here one that is also IMMUTABLE — is removed from the
// mask written back to the handler and its client value is cleared, so the
// protected field cannot be changed through the nested path. Alone, the entry
// leaves no path at all rather than a whole "sub" selection: the update is a
// no-op and the handler gets an empty mask.
func TestServerInterceptor_UpdateMask_NestedOutputOnlyDropped(t *testing.T) {
	t.Parallel()

	reqMD := outputOnlyDescriptor(t)
	resFD := reqMD.Fields().ByName("resource")
	resMD := resFD.Message()
	subFD := resMD.Fields().ByName("sub")
	subMD := subFD.Message()
	maskFD := reqMD.Fields().ByName("update_mask")

	run := func(wantNote bool, paths ...string) (*dynamicpb.Message, []string) {
		sub := dynamicpb.NewMessage(subMD)
		sub.Set(subMD.Fields().ByName("code"), protoreflect.ValueOfString("client"))
		sub.Set(subMD.Fields().ByName("note"), protoreflect.ValueOfString("n"))
		res := dynamicpb.NewMessage(resMD)
		res.Set(subFD, protoreflect.ValueOfMessage(sub))
		req := dynamicpb.NewMessage(reqMD)
		req.Set(resFD, protoreflect.ValueOfMessage(res))
		req.Set(maskFD, protoreflect.ValueOfMessage((&fieldmaskpb.FieldMask{Paths: paths}).ProtoReflect()))

		seenReq, _, err := runUnary(t, newUnary(t), "/x.v1.X/UpdateRes", req, dynamicpb.NewMessage(resMD), nil)
		require.NoError(t, err)
		got, ok := seenReq.(*dynamicpb.Message)
		require.True(t, ok)
		mask, ok := got.Get(maskFD).Message().Interface().(*fieldmaskpb.FieldMask)
		require.True(t, ok)
		gotSub := got.Get(resFD).Message().Get(subFD).Message()
		require.False(t, gotSub.Has(subMD.Fields().ByName("code")), "the client value of the OUTPUT_ONLY field is cleared")
		require.Equal(t, wantNote, gotSub.Has(subMD.Fields().ByName("note")), "the fields the update uses stay")
		return got, mask.GetPaths()
	}

	_, paths := run(true, "sub.code", "sub.note")
	require.Equal(t, []string{"sub.note"}, paths)

	_, paths = run(false, "sub.code")
	require.Empty(t, paths, "an emptied branch disappears instead of selecting the whole sub-message: the update changes nothing")
}

// TestServerInterceptor_EmptyUpdateMask_NestedOutputOnlySkipped pins that the
// explicit empty mask stays empty while OUTPUT_ONLY and unselected mutable
// fields are cleared, including an OUTPUT_ONLY field also marked IMMUTABLE.
func TestServerInterceptor_EmptyUpdateMask_NestedOutputOnlySkipped(t *testing.T) {
	t.Parallel()

	reqMD := outputOnlyDescriptor(t)
	resFD := reqMD.Fields().ByName("resource")
	resMD := resFD.Message()
	subFD := resMD.Fields().ByName("sub")
	subMD := subFD.Message()
	maskFD := reqMD.Fields().ByName("update_mask")

	sub := dynamicpb.NewMessage(subMD)
	sub.Set(subMD.Fields().ByName("code"), protoreflect.ValueOfString("x"))
	sub.Set(subMD.Fields().ByName("note"), protoreflect.ValueOfString("hi"))
	res := dynamicpb.NewMessage(resMD)
	res.Set(subFD, protoreflect.ValueOfMessage(sub))
	req := dynamicpb.NewMessage(reqMD)
	req.Set(resFD, protoreflect.ValueOfMessage(res))
	req.Set(maskFD, protoreflect.ValueOfMessage((&fieldmaskpb.FieldMask{}).ProtoReflect()))

	seenReq, _, err := runUnary(t, newUnary(t), "/x.v1.X/UpdateRes", req, dynamicpb.NewMessage(resMD), nil)
	require.NoError(t, err)
	got, ok := seenReq.(*dynamicpb.Message)
	require.True(t, ok)
	mask, ok := got.Get(maskFD).Message().Interface().(*fieldmaskpb.FieldMask)
	require.True(t, ok)
	require.Empty(t, mask.GetPaths())
	require.False(t, got.Get(resFD).Message().Has(subFD), "output-only and unselected mutable descendants are cleared")
}

// TestServerInterceptor_UpdateMask_OutputOnlyIdentifierCleared pins that an
// IDENTIFIER that is also OUTPUT_ONLY (top-level rid, nested sub.ref, both
// annotation orders) is not preserved in the body the handler sees: its client
// value is cleared, whether or not the mask names it.
func TestServerInterceptor_UpdateMask_OutputOnlyIdentifierCleared(t *testing.T) {
	t.Parallel()

	reqMD := outputOnlyDescriptor(t)
	resFD := reqMD.Fields().ByName("resource")
	resMD := resFD.Message()
	subFD := resMD.Fields().ByName("sub")
	subMD := subFD.Message()
	maskFD := reqMD.Fields().ByName("update_mask")

	for _, paths := range [][]string{{"title", "sub.note"}, {"title", "sub.note", "rid", "sub.ref"}} {
		sub := dynamicpb.NewMessage(subMD)
		sub.Set(subMD.Fields().ByName("ref"), protoreflect.ValueOfString("client"))
		sub.Set(subMD.Fields().ByName("note"), protoreflect.ValueOfString("n"))
		res := dynamicpb.NewMessage(resMD)
		res.Set(subFD, protoreflect.ValueOfMessage(sub))
		res.Set(resMD.Fields().ByName("title"), protoreflect.ValueOfString("t"))
		res.Set(resMD.Fields().ByName("rid"), protoreflect.ValueOfString("client"))
		req := dynamicpb.NewMessage(reqMD)
		req.Set(resFD, protoreflect.ValueOfMessage(res))
		req.Set(maskFD, protoreflect.ValueOfMessage((&fieldmaskpb.FieldMask{Paths: paths}).ProtoReflect()))

		seenReq, _, err := runUnary(t, newUnary(t), "/x.v1.X/UpdateRes", req, dynamicpb.NewMessage(resMD), nil)
		require.NoError(t, err, "%v", paths)
		got, ok := seenReq.(*dynamicpb.Message)
		require.True(t, ok)
		mask, ok := got.Get(maskFD).Message().Interface().(*fieldmaskpb.FieldMask)
		require.True(t, ok)
		require.Equal(t, []string{"sub.note", "title"}, mask.GetPaths(), "%v", paths)

		gotRes := got.Get(resFD).Message()
		require.False(t, gotRes.Has(resMD.Fields().ByName("rid")), "%v: rid keeps no client value", paths)
		require.False(t, gotRes.Get(subFD).Message().Has(subMD.Fields().ByName("ref")), "%v: sub.ref keeps no client value", paths)
	}
}

// TestServerInterceptor_UpdateMask_ServerManagedSubtreesSurvive pins that no
// mask the interceptor hands over can replace a sub-message holding a
// server-managed field. An explicit whole selection is expanded to the
// editable leaves (OUTPUT_ONLY descendants count, also IMMUTABLE or IDENTIFIER
// ones); the synthesized mask
// names populated leaves, also under a sub-message whose OUTPUT_ONLY
// descendant is absent; and an absent parent is left out. Applied to the
// stored resource — by leaf paths, or with Merge for top-level paths — the
// stored server-managed values survive.
func TestServerInterceptor_UpdateMask_ServerManagedSubtreesSurvive(t *testing.T) {
	t.Parallel()

	reqMD := outputOnlyDescriptor(t)
	resFD := reqMD.Fields().ByName("resource")
	resMD := resFD.Message()
	rf := resMD.Fields()
	subMD := rf.ByName("sub").Message()
	boxMD := rf.ByName("box").Message()
	extraMD := boxMD.Fields().ByName("extra").Message()
	maskFD := reqMD.Fields().ByName("update_mask")

	set := func(m *dynamicpb.Message, name, v string) {
		m.Set(m.Descriptor().Fields().ByName(protoreflect.Name(name)), protoreflect.ValueOfString(v))
	}
	stored := func() *dynamicpb.Message {
		sub := dynamicpb.NewMessage(subMD)
		set(sub, "code", "server")
		set(sub, "note", "old")
		set(sub, "ref", "server")
		extra := dynamicpb.NewMessage(extraMD)
		set(extra, "when", "server")
		box := dynamicpb.NewMessage(boxMD)
		set(box, "label", "old")
		box.Set(boxMD.Fields().ByName("extra"), protoreflect.ValueOfMessage(extra))
		res := dynamicpb.NewMessage(resMD)
		res.Set(rf.ByName("sub"), protoreflect.ValueOfMessage(sub))
		res.Set(rf.ByName("box"), protoreflect.ValueOfMessage(box))
		set(res, "title", "old")
		set(res, "rid", "server")
		return res
	}
	call := func(res *dynamicpb.Message, paths []string, opts ...fieldmask.Option) (*dynamicpb.Message, []string, error) {
		req := dynamicpb.NewMessage(reqMD)
		req.Set(resFD, protoreflect.ValueOfMessage(res))
		if paths != nil {
			req.Set(maskFD, protoreflect.ValueOfMessage((&fieldmaskpb.FieldMask{Paths: paths}).ProtoReflect()))
		}
		seenReq, _, err := runUnary(t, newUnary(t, opts...), "/x.v1.X/UpdateRes", req, dynamicpb.NewMessage(resMD), nil)
		if err != nil {
			return nil, nil, err
		}
		got, ok := seenReq.(*dynamicpb.Message)
		require.True(t, ok)
		mask, ok := got.Get(maskFD).Message().Interface().(*fieldmaskpb.FieldMask)
		require.True(t, ok)
		body, ok := got.Get(resFD).Message().Interface().(*dynamicpb.Message)
		require.True(t, ok)
		return body, mask.GetPaths(), nil
	}
	assertServerValues := func(dst *dynamicpb.Message) {
		t.Helper()
		sub := dst.Get(rf.ByName("sub")).Message()
		require.Equal(t, "server", sub.Get(subMD.Fields().ByName("code")).String())
		require.Equal(t, "server", sub.Get(subMD.Fields().ByName("ref")).String())
		extra := dst.Get(rf.ByName("box")).Message().Get(boxMD.Fields().ByName("extra")).Message()
		require.Equal(t, "server", extra.Get(extraMD.Fields().ByName("when")).String())
		require.Equal(t, "server", dst.Get(rf.ByName("rid")).String())
	}

	// Explicit whole selections are expanded to the editable leaves.
	for path, want := range map[string][]string{"sub": {"sub.note"}, "box": {"box.label"}} {
		body := dynamicpb.NewMessage(resMD)
		set(body, "title", "t")
		gotBody, paths, err := call(body, []string{path})
		require.NoError(t, err, path)
		require.Equal(t, want, paths, path)
		dst := stored()
		testhelpers.ApplyLeafPaths(t, dst, gotBody, paths)
		assertServerValues(dst)
	}

	// Synthesized mask: populated leaves only.
	body := dynamicpb.NewMessage(resMD)
	set(body, "title", "new")
	sub := dynamicpb.NewMessage(subMD)
	set(sub, "note", "new")
	body.Set(rf.ByName("sub"), protoreflect.ValueOfMessage(sub))
	box := dynamicpb.NewMessage(boxMD)
	set(box, "label", "new")
	body.Set(rf.ByName("box"), protoreflect.ValueOfMessage(box))
	gotBody, paths, err := call(body, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"box.label", "sub.note", "title"}, paths)
	dst := stored()
	testhelpers.ApplyLeafPaths(t, dst, gotBody, paths)
	assertServerValues(dst)
	require.Equal(t, "new", dst.Get(rf.ByName("sub")).Message().Get(subMD.Fields().ByName("note")).String())
	require.Equal(t, "new", dst.Get(rf.ByName("box")).Message().Get(boxMD.Fields().ByName("label")).String())

	// Absent parents: left out of the synthesized mask; Merge keeps them.
	body = dynamicpb.NewMessage(resMD)
	set(body, "title", "new")
	gotBody, paths, err = call(body, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"title"}, paths)
	dst = stored()
	require.NoError(t, update.Merge(pbfieldmask.FromPaths(paths...), dst, gotBody))
	assertServerValues(dst)
	require.Equal(t, "new", dst.Get(rf.ByName("title")).String())
}

// TestServerInterceptor_EmptyUpdateMask_UnselectedContainersDropped verifies prepared no-op sanitization.
// An explicit empty mask authorizes no writes and keeps only the resource routing identity.
func TestServerInterceptor_EmptyUpdateMask_UnselectedContainersDropped(t *testing.T) {
	t.Parallel()

	reqMD := outputOnlyDescriptor(t)
	resFD := reqMD.Fields().ByName("resource")
	resMD := resFD.Message()
	subFD := resMD.Fields().ByName("sub")
	subMD := subFD.Message()
	maskFD := reqMD.Fields().ByName("update_mask")

	sub := dynamicpb.NewMessage(subMD)
	sub.Set(subMD.Fields().ByName("code"), protoreflect.ValueOfString("client"))
	sub.Set(subMD.Fields().ByName("ref"), protoreflect.ValueOfString("client"))
	res := dynamicpb.NewMessage(resMD)
	res.Set(subFD, protoreflect.ValueOfMessage(sub))
	req := dynamicpb.NewMessage(reqMD)
	req.Set(resFD, protoreflect.ValueOfMessage(res))
	req.Set(maskFD, protoreflect.ValueOfMessage((&fieldmaskpb.FieldMask{}).ProtoReflect()))

	seenReq, _, err := runUnary(t, newUnary(t), "/x.v1.X/UpdateRes", req, dynamicpb.NewMessage(resMD), nil)
	require.NoError(t, err)
	got, ok := seenReq.(*dynamicpb.Message)
	require.True(t, ok)
	mask, ok := got.Get(maskFD).Message().Interface().(*fieldmaskpb.FieldMask)
	require.True(t, ok)
	require.Empty(t, mask.GetPaths())
	require.False(t, got.Get(resFD).Message().Has(subFD), "OUTPUT_ONLY input is cleared")

	// A top-level identifier on its own stays in the body: it addresses the
	// resource.
	idReq := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Id: "id-1", Profile: &pb.Profile{Id: "p-1", UpdatedAt: "client"}},
		UpdateMask: &fieldmaskpb.FieldMask{},
	}
	handlerCalled := false
	_, err = newUnary(t)(t.Context(), idReq, &grpc.UnaryServerInfo{FullMethod: "/x.v1.X/UpdateResource"}, func(ctx context.Context, request any) (any, error) {
		handlerCalled = true
		gotReq, ok := request.(*pb.UpdateResourceRequest)
		require.True(t, ok)
		require.NotNil(t, gotReq.UpdateMask, "an explicit empty mask must not become an omitted mask")
		require.Empty(t, gotReq.UpdateMask.Paths)
		require.Equal(t, "id-1", gotReq.GetResource().GetId())
		require.Nil(t, gotReq.GetResource().GetProfile(), "an unselected container is dropped even with a nested identifier")
		require.True(t, proto.Equal(&pb.Resource{Id: "id-1"}, gotReq.Resource))
		pending, ok := update.PlanFromContext(ctx, gotReq.Resource)
		require.True(t, ok)
		require.Empty(t, pending.Mask())
		require.Empty(t, pending.Operations())
		mask, err := pending.Complete(&pb.Resource{Id: "id-1", Profile: &pb.Profile{Id: "stored"}})
		require.NoError(t, err)
		require.Empty(t, mask)
		return &pb.Resource{}, nil
	})
	require.NoError(t, err)
	require.True(t, handlerCalled)
}
