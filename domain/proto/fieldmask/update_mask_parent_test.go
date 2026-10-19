// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// TestApplyUpdateMask_ParentOfIdentifierRejected pins that naming a message
// field whole, when it holds an IDENTIFIER, is rejected: the whole-field
// selection would let the handler replace or clear the parent and with it the
// identifier the update must not change (AIP-203). Populated and absent parents
// are rejected alike; naming the parent's other fields is accepted.
func TestApplyUpdateMask_ParentOfIdentifierRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  *pb.Resource
	}{
		{name: "populated parent", msg: &pb.Resource{Profile: &pb.Profile{Id: "replacement", DisplayName: "Ann"}}},
		{name: "parent without identifier", msg: &pb.Resource{Profile: &pb.Profile{DisplayName: "Ann"}}},
		{name: "absent parent", msg: &pb.Resource{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			before := proto.Clone(tt.msg)
			err := fieldmask.FromPaths("profile").ApplyUpdateMask(tt.msg)

			behaviorErr, ok := errors.AsType[*fieldmask.UpdateMaskBehaviorError](err)
			require.True(t, ok, "want *UpdateMaskBehaviorError, got %v", err)
			// Every server-managed descendant is reported: the identifier and
			// the OUTPUT_ONLY updated_at.
			require.Len(t, behaviorErr.Violations, 2)
			require.Equal(t, "profile.id", behaviorErr.Violations[0].Path)
			require.Equal(t, annotations.FieldBehavior_IDENTIFIER, behaviorErr.Violations[0].Behavior)
			require.Equal(t, "profile.updated_at", behaviorErr.Violations[1].Path)
			require.Equal(t, annotations.FieldBehavior_OUTPUT_ONLY, behaviorErr.Violations[1].Behavior)
			require.True(t, proto.Equal(before, tt.msg), "a rejected mask leaves the message untouched")
		})
	}

	msg := &pb.Resource{Profile: &pb.Profile{Id: "p-1", DisplayName: "Ann"}}
	mask := fieldmask.FromPaths("profile.display_name")
	require.NoError(t, mask.ApplyUpdateMask(msg))
	require.Equal(t, "p-1", msg.GetProfile().GetId(), "the nested identifier is preserved")
	require.Equal(t, "Ann", msg.GetProfile().GetDisplayName())
}

// outerDescriptor builds:
//
//	message Inner { string code = 1 [IMMUTABLE]; string note = 2; Inner child = 3; repeated Leaf leaves = 4;
//	                string slug = 5 [REQUIRED, IMMUTABLE]; }
//	message Leaf  { string id = 1 [IDENTIFIER]; }
//	message Plain { string note = 1; }
//	message Outer { Inner inner = 1; Plain plain = 2; Wrap wrap = 3; }
//	message Wrap  { Inner inner = 1; }
func outerDescriptor(tb testing.TB) protoreflect.MessageDescriptor {
	tb.Helper()

	behaviorOpts := func(b ...annotations.FieldBehavior) *descriptorpb.FieldOptions {
		o := &descriptorpb.FieldOptions{}
		proto.SetExtension(o, annotations.E_FieldBehavior, b)
		return o
	}
	msg := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	rep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	field := func(name string, num int32, typ descriptorpb.FieldDescriptorProto_Type, label descriptorpb.FieldDescriptorProto_Label,
		typeName string, opts *descriptorpb.FieldOptions,
	) *descriptorpb.FieldDescriptorProto {
		f := &descriptorpb.FieldDescriptorProto{Name: new(name), Number: new(num), Type: &typ, Label: &label, Options: opts}
		if typeName != "" {
			f.TypeName = new(typeName)
		}
		return f
	}

	fdp := &descriptorpb.FileDescriptorProto{
		Name:    new("fieldmask_parent_test.proto"),
		Package: new("fieldmaskparenttest"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: new("Inner"), Field: []*descriptorpb.FieldDescriptorProto{
				field("code", 1, str, opt, "", behaviorOpts(annotations.FieldBehavior_IMMUTABLE)),
				field("note", 2, str, opt, "", nil),
				field("child", 3, msg, opt, ".fieldmaskparenttest.Inner", nil),
				field("leaves", 4, msg, rep, ".fieldmaskparenttest.Leaf", nil),
				field("slug", 5, str, opt, "", behaviorOpts(annotations.FieldBehavior_REQUIRED, annotations.FieldBehavior_IMMUTABLE)),
			}},
			{Name: new("Leaf"), Field: []*descriptorpb.FieldDescriptorProto{
				field("id", 1, str, opt, "", behaviorOpts(annotations.FieldBehavior_IDENTIFIER)),
			}},
			{Name: new("Plain"), Field: []*descriptorpb.FieldDescriptorProto{
				field("note", 1, str, opt, "", nil),
			}},
			{Name: new("Wrap"), Field: []*descriptorpb.FieldDescriptorProto{
				field("inner", 1, msg, opt, ".fieldmaskparenttest.Inner", nil),
			}},
			{Name: new("Outer"), Field: []*descriptorpb.FieldDescriptorProto{
				field("inner", 1, msg, opt, ".fieldmaskparenttest.Inner", nil),
				field("plain", 2, msg, opt, ".fieldmaskparenttest.Plain", nil),
				field("wrap", 3, msg, opt, ".fieldmaskparenttest.Wrap", nil),
			}},
		},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	require.NoError(tb, err)
	return fd.Messages().ByName("Outer")
}

// TestApplyUpdateMask_ParentOfImmutableRejected pins the same rule for an
// IMMUTABLE descendant — also one that is REQUIRED too — at any depth of
// singular sub-messages, on a recursive message type (the walk terminates).
// Repeated and map fields are replaced as a whole by design and are not
// descended; a message without protected descendants may still be named whole.
func TestApplyUpdateMask_ParentOfImmutableRejected(t *testing.T) {
	t.Parallel()

	md := outerDescriptor(t)
	tests := []struct {
		name      string
		paths     []string
		wantPaths []string
	}{
		{name: "direct parent", paths: []string{"inner"}, wantPaths: []string{"inner.code", "inner.slug"}},
		{name: "grandparent", paths: []string{"wrap"}, wantPaths: []string{"wrap.inner.code", "wrap.inner.slug"}},
		{name: "parent named under a sub-mask", paths: []string{"wrap.inner"}, wantPaths: []string{"wrap.inner.code", "wrap.inner.slug"}},
		{name: "recursive child", paths: []string{"inner.child"}, wantPaths: []string{"inner.child.code", "inner.child.slug"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := fieldmask.FromPaths(tt.paths...).ApplyUpdateMask(dynamicpb.NewMessage(md))
			behaviorErr, ok := errors.AsType[*fieldmask.UpdateMaskBehaviorError](err)
			require.True(t, ok, "want *UpdateMaskBehaviorError, got %v", err)
			var got []string
			for _, v := range behaviorErr.Violations {
				require.Equal(t, annotations.FieldBehavior_IMMUTABLE, v.Behavior)
				got = append(got, v.Path)
			}
			require.Equal(t, tt.wantPaths, got)
		})
	}

	for _, paths := range [][]string{{"plain"}, {"inner.note"}, {"inner.leaves"}, {"wrap.inner.note"}} {
		require.NoError(t, fieldmask.FromPaths(paths...).ApplyUpdateMask(dynamicpb.NewMessage(md)), "paths %v", paths)
	}
}

// multiDescriptor builds fields carrying several behaviors, in both
// declaration orders, at the top level and in a sub-message:
//
//	message Multi {
//	  string req_imm = 1 [REQUIRED, IMMUTABLE];
//	  string imm_req = 2 [IMMUTABLE, REQUIRED];
//	  string req_id  = 3 [REQUIRED, IDENTIFIER];
//	  string req_out = 4 [REQUIRED, OUTPUT_ONLY];
//	  Multi  sub     = 5;
//	  string imm_out = 6 [IMMUTABLE, OUTPUT_ONLY];
//	  string out_imm = 7 [OUTPUT_ONLY, IMMUTABLE];
//	  string note    = 8;
//	  string id_out  = 9 [IDENTIFIER, OUTPUT_ONLY];
//	  string out_id  = 10 [OUTPUT_ONLY, IDENTIFIER];
//	}
func multiDescriptor(tb testing.TB) protoreflect.MessageDescriptor {
	tb.Helper()

	behaviorOpts := func(b ...annotations.FieldBehavior) *descriptorpb.FieldOptions {
		o := &descriptorpb.FieldOptions{}
		proto.SetExtension(o, annotations.E_FieldBehavior, b)
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
	req, imm, id, out := annotations.FieldBehavior_REQUIRED, annotations.FieldBehavior_IMMUTABLE,
		annotations.FieldBehavior_IDENTIFIER, annotations.FieldBehavior_OUTPUT_ONLY

	fdp := &descriptorpb.FileDescriptorProto{
		Name:    new("fieldmask_multi_test.proto"),
		Package: new("fieldmaskmultitest"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: new("Multi"), Field: []*descriptorpb.FieldDescriptorProto{
				field("req_imm", 1, str, "", behaviorOpts(req, imm)),
				field("imm_req", 2, str, "", behaviorOpts(imm, req)),
				field("req_id", 3, str, "", behaviorOpts(req, id)),
				field("req_out", 4, str, "", behaviorOpts(req, out)),
				field("sub", 5, msg, ".fieldmaskmultitest.Multi", nil),
				field("imm_out", 6, str, "", behaviorOpts(imm, out)),
				field("out_imm", 7, str, "", behaviorOpts(out, imm)),
				field("note", 8, str, "", nil),
				field("id_out", 9, str, "", behaviorOpts(id, out)),
				field("out_id", 10, str, "", behaviorOpts(out, id)),
			}},
		},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	require.NoError(tb, err)
	return fd.Messages().ByName("Multi")
}

// TestApplyUpdateMask_MultiBehaviorPrecedence pins that a field carrying
// several behaviors is judged by presence with a fixed precedence —
// OUTPUT_ONLY, then IDENTIFIER, then IMMUTABLE, then REQUIRED — rather than by
// its first annotation: a REQUIRED field that is also IMMUTABLE or IDENTIFIER
// cannot be modified by naming it, set or not, and the violation reports the
// protecting behavior. Top-level and nested paths behave alike.
func TestApplyUpdateMask_MultiBehaviorPrecedence(t *testing.T) {
	t.Parallel()

	md := multiDescriptor(t)
	newMsg := func(set bool) *dynamicpb.Message {
		m := dynamicpb.NewMessage(md)
		sub := dynamicpb.NewMessage(md)
		if set {
			for _, name := range []string{"req_imm", "imm_req", "req_id", "req_out"} {
				m.Set(md.Fields().ByName(protoreflect.Name(name)), protoreflect.ValueOfString("v"))
				sub.Set(md.Fields().ByName(protoreflect.Name(name)), protoreflect.ValueOfString("v"))
			}
		}
		m.Set(md.Fields().ByName("sub"), protoreflect.ValueOfMessage(sub))
		return m
	}
	tests := []struct {
		path string
		want annotations.FieldBehavior
	}{
		{path: "req_imm", want: annotations.FieldBehavior_IMMUTABLE},
		{path: "imm_req", want: annotations.FieldBehavior_IMMUTABLE},
		{path: "req_id", want: annotations.FieldBehavior_IDENTIFIER},
		{path: "sub.req_imm", want: annotations.FieldBehavior_IMMUTABLE},
		{path: "sub.imm_req", want: annotations.FieldBehavior_IMMUTABLE},
		{path: "sub.req_id", want: annotations.FieldBehavior_IDENTIFIER},
	}
	for _, tt := range tests {
		for _, set := range []bool{true, false} {
			err := fieldmask.FromPaths(tt.path).ApplyUpdateMask(newMsg(set))
			behaviorErr, ok := errors.AsType[*fieldmask.UpdateMaskBehaviorError](err)
			require.True(t, ok, "%s (set=%v): want *UpdateMaskBehaviorError, got %v", tt.path, set, err)
			require.Len(t, behaviorErr.Violations, 1, tt.path)
			require.Equal(t, tt.path, behaviorErr.Violations[0].Path)
			require.Equal(t, tt.want, behaviorErr.Violations[0].Behavior, "%s (set=%v)", tt.path, set)
		}
	}

	// OUTPUT_ONLY wins over REQUIRED and IMMUTABLE: the entry is dropped from
	// the mask — nested ones too — and the client value is cleared, not
	// reported. An emptied branch disappears instead of becoming a
	// whole-parent selection.
	for _, name := range []string{"req_out", "imm_out", "out_imm"} {
		for _, prefix := range []string{"", "sub."} {
			path := prefix + name
			for _, set := range []bool{true, false} {
				m := newMsg(set)
				if set {
					m.Get(md.Fields().ByName("sub")).Message().Set(md.Fields().ByName("note"), protoreflect.ValueOfString("n"))
				}
				mask := fieldmask.FromPaths(path, prefix+"note")
				require.NoError(t, mask.ApplyUpdateMask(m), "%s (set=%v)", path, set)
				require.Equal(t, []string{prefix + "note"}, withoutIdentifiers(mask.ToPaths()), "%s (set=%v)", path, set)

				target := m
				if prefix != "" {
					target = m.Get(md.Fields().ByName("sub")).Message().Interface().(*dynamicpb.Message)
				}
				require.False(t, target.Has(md.Fields().ByName(protoreflect.Name(name))),
					"%s (set=%v): the client value of an OUTPUT_ONLY field is cleared", path, set)
			}

			mask := fieldmask.FromPaths(path)
			require.NoError(t, mask.ApplyUpdateMask(newMsg(true)), path)
			require.Empty(t, withoutIdentifiers(mask.ToPaths()), "%s alone: the emptied branch disappears", path)
		}
	}
}

// withoutIdentifiers drops the IDENTIFIER entries ApplyUpdateMask adds to keep
// set identifiers in the body.
func withoutIdentifiers(paths []string) []string {
	out := []string{}
	for _, p := range paths {
		if p != "req_id" && p != "sub.req_id" {
			out = append(out, p)
		}
	}
	return out
}

// TestApplyUpdateMask_OutputOnlyOnlyBranchDropped pins that a branch naming
// only OUTPUT_ONLY fields disappears from the mask even when its sub-message
// carries a set IDENTIFIER: OUTPUT_ONLY entries are removed before identifiers
// are preserved, so the identifier does not keep an otherwise empty branch
// alive.
func TestApplyUpdateMask_OutputOnlyOnlyBranchDropped(t *testing.T) {
	t.Parallel()

	msg := &pb.Resource{Profile: &pb.Profile{Id: "p1", UpdatedAt: "ts"}}
	mask := fieldmask.FromPaths("profile.updated_at")
	require.NoError(t, mask.ApplyUpdateMask(msg))
	require.Empty(t, mask.ToPaths())
	require.False(t, mask.Contains("profile"), "no empty branch is left behind")
	require.Empty(t, msg.GetProfile().GetUpdatedAt(), "the client value of the OUTPUT_ONLY field is cleared")
}

// wholeSubDescriptor builds:
//
//	message Deep { string stamp = 1 [OUTPUT_ONLY, IMMUTABLE]; string v = 2; }
//	message Sub  { string code = 1 [IMMUTABLE, OUTPUT_ONLY]; string note = 2; Deep deep = 3; }
//	message Res  { Sub sub = 1; }
func wholeSubDescriptor(tb testing.TB) protoreflect.MessageDescriptor {
	tb.Helper()

	behaviorOpts := func(b ...annotations.FieldBehavior) *descriptorpb.FieldOptions {
		o := &descriptorpb.FieldOptions{}
		proto.SetExtension(o, annotations.E_FieldBehavior, b)
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
	imm, out := annotations.FieldBehavior_IMMUTABLE, annotations.FieldBehavior_OUTPUT_ONLY

	fdp := &descriptorpb.FileDescriptorProto{
		Name:    new("fieldmask_wholesub_test.proto"),
		Package: new("fieldmaskwholesubtest"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: new("Deep"), Field: []*descriptorpb.FieldDescriptorProto{
				field("stamp", 1, str, "", behaviorOpts(out, imm)),
				field("v", 2, str, "", nil),
			}},
			{Name: new("Sub"), Field: []*descriptorpb.FieldDescriptorProto{
				field("code", 1, str, "", behaviorOpts(imm, out)),
				field("note", 2, str, "", nil),
				field("deep", 3, msg, ".fieldmaskwholesubtest.Deep", nil),
			}},
			{Name: new("Res"), Field: []*descriptorpb.FieldDescriptorProto{
				field("sub", 1, msg, ".fieldmaskwholesubtest.Sub", nil),
			}},
		},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	require.NoError(tb, err)
	return fd.Messages().ByName("Res")
}

// TestApplyUpdateMask_WholeMessageOutputOnlyDescendants pins that a
// whole-message selection is rejected when its singular subtree holds an
// OUTPUT_ONLY field (here also IMMUTABLE, in both orders, at two depths):
// replacing the message would delete the stored server-managed value, so the
// violation is reported as OUTPUT_ONLY and nothing is applied. Naming the
// editable leaves instead is accepted and leaves the stored values alone.
func TestApplyUpdateMask_WholeMessageOutputOnlyDescendants(t *testing.T) {
	t.Parallel()

	resMD := wholeSubDescriptor(t)
	subFD := resMD.Fields().ByName("sub")
	subMD := subFD.Message()
	deepFD := subMD.Fields().ByName("deep")
	deepMD := deepFD.Message()

	newRes := func(code, note, stamp, v string) *dynamicpb.Message {
		deep := dynamicpb.NewMessage(deepMD)
		if stamp != "" {
			deep.Set(deepMD.Fields().ByName("stamp"), protoreflect.ValueOfString(stamp))
		}
		deep.Set(deepMD.Fields().ByName("v"), protoreflect.ValueOfString(v))
		sub := dynamicpb.NewMessage(subMD)
		if code != "" {
			sub.Set(subMD.Fields().ByName("code"), protoreflect.ValueOfString(code))
		}
		sub.Set(subMD.Fields().ByName("note"), protoreflect.ValueOfString(note))
		sub.Set(deepFD, protoreflect.ValueOfMessage(deep))
		res := dynamicpb.NewMessage(resMD)
		res.Set(subFD, protoreflect.ValueOfMessage(sub))
		return res
	}

	// Explicit whole selection: rejected, populated or absent.
	for name, req := range map[string]*dynamicpb.Message{
		"populated": newRes("client", "new", "client", "new"),
		"absent":    dynamicpb.NewMessage(resMD),
	} {
		before := proto.Clone(req)
		err := fieldmask.FromPaths("sub").ApplyUpdateMask(req)
		behaviorErr, ok := errors.AsType[*fieldmask.UpdateMaskBehaviorError](err)
		require.True(t, ok, "%s: want *UpdateMaskBehaviorError, got %v", name, err)
		var got []string
		for _, v := range behaviorErr.Violations {
			require.Equal(t, annotations.FieldBehavior_OUTPUT_ONLY, v.Behavior, name)
			got = append(got, v.Path)
		}
		require.Equal(t, []string{"sub.code", "sub.deep.stamp"}, got, name)
		require.True(t, proto.Equal(before, req), "%s: a rejected mask leaves the message untouched", name)
	}

	// Leaf paths are accepted; applied to the stored resource they keep the
	// stored server-managed values.
	stored := newRes("server", "old", "server", "old")
	req := newRes("", "new", "", "new")
	mask := fieldmask.FromPaths("sub.note", "sub.deep.v")
	require.NoError(t, mask.ApplyUpdateMask(req))
	testhelpers.ApplyLeafPaths(t, stored, req, mask.ToPaths())
	gotSub := stored.Get(subFD).Message()
	require.Equal(t, "server", gotSub.Get(subMD.Fields().ByName("code")).String())
	require.Equal(t, "new", gotSub.Get(subMD.Fields().ByName("note")).String())
	gotDeep := gotSub.Get(deepFD).Message()
	require.Equal(t, "server", gotDeep.Get(deepMD.Fields().ByName("stamp")).String())
	require.Equal(t, "new", gotDeep.Get(deepMD.Fields().ByName("v")).String())
}

// TestApplyUpdateMask_OutputOnlyIdentifierNotPreserved pins that an
// IDENTIFIER that is also OUTPUT_ONLY, in either annotation order, is not
// preserved: OUTPUT_ONLY dominates, so the client value is cleared and the
// field never re-enters the mask — whether the mask names it or not, at the
// top level and nested.
func TestApplyUpdateMask_OutputOnlyIdentifierNotPreserved(t *testing.T) {
	t.Parallel()

	md := multiDescriptor(t)
	fields := md.Fields()
	for _, prefix := range []string{"", "sub."} {
		for _, extra := range [][]string{nil, {"id_out", "out_id"}} {
			m := dynamicpb.NewMessage(md)
			sub := dynamicpb.NewMessage(md)
			for _, target := range []*dynamicpb.Message{m, sub} {
				target.Set(fields.ByName("id_out"), protoreflect.ValueOfString("client"))
				target.Set(fields.ByName("out_id"), protoreflect.ValueOfString("client"))
				target.Set(fields.ByName("note"), protoreflect.ValueOfString("n"))
			}
			m.Set(fields.ByName("sub"), protoreflect.ValueOfMessage(sub))

			paths := []string{prefix + "note"}
			for _, e := range extra {
				paths = append(paths, prefix+e)
			}
			mask := fieldmask.FromPaths(paths...)
			require.NoError(t, mask.ApplyUpdateMask(m), "%v", paths)
			require.Equal(t, []string{prefix + "note"}, mask.ToPaths(), "%v", paths)

			target := m
			if prefix != "" {
				target = m.Get(fields.ByName("sub")).Message().Interface().(*dynamicpb.Message)
			}
			for _, name := range []protoreflect.Name{"id_out", "out_id"} {
				require.False(t, target.Has(fields.ByName(name)), "%v: %s keeps no client value", paths, name)
			}
		}
	}
}
