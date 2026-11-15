// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldbehavior_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/fieldmask/extract"
	"github.com/altessa-s/go-aip/fieldmask/update"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldbehavior"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/updatemask"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pbfieldbehavior "github.com/altessa-s/go-aip/fieldbehavior"
	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
	badrequestv1 "github.com/altessa-s/proto-gen-go/io/altessa/badrequest/v1"
)

// chain runs fieldbehavior before fieldmask, the order the gRPC server
// factory installs them in, around a handler that records the request.
// fieldbehavior applies update masks with the normalizer of fmOpts, as the
// factory gives it.
func chain(t *testing.T, fbOpts []fieldbehavior.Option, fmOpts []fieldmask.Option) func(method string, req any) (any, error) {
	t.Helper()

	fbOpts = append([]fieldbehavior.Option{fieldbehavior.WithUpdateNormalizer(fieldmask.UpdateNormalizer(fmOpts...))}, fbOpts...)
	fb := fieldbehavior.ServerInterceptor(fbOpts...).ServerUnaryInterceptor()
	fm := fieldmask.ServerInterceptor(fmOpts...).ServerUnaryInterceptor()

	return func(method string, req any) (any, error) {
		info := &grpc.UnaryServerInfo{FullMethod: method}
		var seen any
		handler := func(_ context.Context, r any) (any, error) {
			seen = r
			return &pb.Resource{}, nil
		}
		_, err := fb(t.Context(), req, info, func(ctx context.Context, r any) (any, error) {
			return fm(ctx, r, info, handler)
		})
		return seen, err
	}
}

// violationPaths returns the field paths of the BadRequest detail of err.
func violationPaths(t *testing.T, err error) []string {
	t.Helper()

	st, ok := status.FromError(err)
	require.True(t, ok)
	var paths []string
	for _, d := range st.Details() {
		if br, ok := d.(*badrequestv1.BadRequest); ok {
			for _, fv := range br.GetFieldViolations() {
				paths = append(paths, fv.GetFieldPath())
			}
		}
	}
	return paths
}

// TestUpdate_ProtectedValuesDeferred verifies the v0.1.5 contract.
// Selected protected values reach the handler; malformed paths still fail at the boundary.
func TestUpdate_ProtectedValuesDeferred(t *testing.T) {
	t.Parallel()

	const method = "/x.v1.X/UpdateResource"
	req := func(immutable bool, paths ...string) *pb.UpdateResourceRequest {
		var mask *fieldmaskpb.FieldMask
		if paths != nil {
			mask = &fieldmaskpb.FieldMask{Paths: paths}
		}
		res := &pb.Resource{Id: "id-1", Name: "n", Description: "d", Profile: &pb.Profile{DisplayName: "p"}}
		if immutable {
			res.TenantId, res.Slug = "other", "other"
		}
		return &pb.UpdateResourceRequest{Resource: res, UpdateMask: mask}
	}
	chains := map[string][]fieldmask.Option{
		"fieldmask enabled":         nil,
		"method excluded from mask": {fieldmask.WithIgnoreMethods(method)},
		"method kind skipped":       {fieldmask.WithMethodKind(method, fieldmask.KindSkip)},
	}

	for name, fmOpts := range chains {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			run := chain(t, nil, fmOpts)

			for _, paths := range [][]string{
				{"description", "aliases.0", "tenant_id"}, {"description", "description.no_such"}, {"*", "*"}, {""},
			} {
				seen, err := run(method, req(true, paths...))
				require.Equal(t, codes.InvalidArgument, status.Code(err), "%q: %v", paths, err)
				require.Nil(t, seen, "%q: the handler must not run", paths)
			}

			for _, tc := range []struct {
				paths []string
				want  []string
			}{
				{paths: []string{"description"}, want: []string{"description"}},
				{paths: []string{"profile"}, want: []string{"profile.display_name", "profile.secret"}}, // profile.id is an IDENTIFIER
				{paths: []string{}, want: nil},
			} {
				seen, err := run(method, req(true, tc.paths...))
				require.NoError(t, err, "%q", tc.paths)
				got := seen.(*pb.UpdateResourceRequest)
				require.Equal(t, tc.want, got.GetUpdateMask().GetPaths(), "%q: effective mask", tc.paths)
				require.Empty(t, got.GetResource().GetTenantId(), "%q: unselected IMMUTABLE cleared", tc.paths)
				require.Empty(t, got.GetResource().GetSlug(), "%q: unselected IMMUTABLE cleared", tc.paths)
			}

			// "*" passes when the request sets no IMMUTABLE value; the mask
			// written back names no IMMUTABLE field.
			seen, err := run(method, req(false, "*"))
			require.NoError(t, err)
			mask := seen.(*pb.UpdateResourceRequest).GetUpdateMask().GetPaths()
			require.Contains(t, mask, "description")
			require.NotContains(t, mask, "tenant_id")
			require.NotContains(t, mask, "slug")

			// An omitted mask implies populated fields and is written back.
			seen, err = run(method, req(false))
			require.NoError(t, err)
			require.NotNil(t, seen.(*pb.UpdateResourceRequest).GetUpdateMask())
		})
	}
}

// TestUpdate_ProtectedValuesDeferredWithoutFieldMask verifies the v0.1.5 contract.
// The fieldbehavior interceptor alone retains selected protected values and captures comparison input.
func TestUpdate_ProtectedValuesDeferredWithoutFieldMask(t *testing.T) {
	t.Parallel()

	uni := newUnary(t)
	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Id: "id-1", TenantId: "other"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"tenant_id"}},
	}
	seen, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NotNil(t, seen)
	require.NoError(t, err)
	require.Equal(t, "other", req.Resource.TenantId)

	// A narrow mask clears unselected immutable input; the plan owns comparisons.
	req = &pb.UpdateResourceRequest{Resource: fullResource(), UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"description"}}}
	seenReq, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)
	got := seenReq.(*pb.UpdateResourceRequest)
	require.Empty(t, got.GetResource().GetTenantId())
	require.Equal(t, "desc", got.GetResource().GetDescription())
	require.Equal(t, "id-1", got.GetResource().GetId())
	require.Equal(t, []string{"description"}, got.GetUpdateMask().GetPaths())

	// An explicit empty mask changes nothing: it is written back empty.
	req = &pb.UpdateResourceRequest{Resource: &pb.Resource{TenantId: "other"}, UpdateMask: &fieldmaskpb.FieldMask{}}
	seenReq, _, err = runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)
	require.NotNil(t, seenReq.(*pb.UpdateResourceRequest).GetUpdateMask())
	require.Empty(t, seenReq.(*pb.UpdateResourceRequest).GetUpdateMask().GetPaths())
}

// TestUpdate_ExtractorMissPreservesImmutable verifies the v0.1.5 contract.
// A lookup miss preserves protected values through the StripUpdate fallback.
func TestUpdate_ExtractorMissPreservesImmutable(t *testing.T) {
	t.Parallel()

	uni := newUnary(t)
	req := &pb.NestedUpdateResourceRequest{
		Resource: &pb.Resource{TenantId: "other", Description: "d"},
		Options:  &pb.NestedUpdateResourceRequest_Options{UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"description"}}},
	}
	seenReq, _, err := runUnary(t, uni, "/x.v1.X/UpdateNestedResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)
	require.NotNil(t, seenReq)
	require.Equal(t, "other", req.Resource.TenantId)
}

// TestUpdate_SharedMaskExtraction verifies the v0.1.5 contract.
// Nested and renamed masks share extraction and filter unselected immutable input.
func TestUpdate_SharedMaskExtraction(t *testing.T) {
	t.Parallel()

	t.Run("nested mask", func(t *testing.T) {
		t.Parallel()

		const method = "/x.v1.X/UpdateNestedResource"
		fmOpts := []fieldmask.Option{fieldmask.WithUpdateExtractor(method, extract.NewUpdateFunc(
			func(r *pb.NestedUpdateResourceRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetUpdateMask() },
			func(r *pb.NestedUpdateResourceRequest) *pb.Resource { return r.GetResource() },
			func(r *pb.NestedUpdateResourceRequest, m *fieldmaskpb.FieldMask) { r.Options.UpdateMask = m },
		))}
		run := chain(t, nil, fmOpts)
		req := func(paths ...string) *pb.NestedUpdateResourceRequest {
			return &pb.NestedUpdateResourceRequest{
				Resource: &pb.Resource{Name: "n", TenantId: "other", Description: "d"},
				Options:  &pb.NestedUpdateResourceRequest_Options{UpdateMask: &fieldmaskpb.FieldMask{Paths: paths}},
			}
		}

		seen, err := run(method, req("tenant_id"))
		require.NoError(t, err)
		require.NotNil(t, seen)

		seen, err = run(method, req("description"))
		require.NoError(t, err)
		got := seen.(*pb.NestedUpdateResourceRequest)
		require.Empty(t, got.GetResource().GetTenantId())
		require.Equal(t, []string{"description"}, got.GetOptions().GetUpdateMask().GetPaths())
	})

	t.Run("renamed mask", func(t *testing.T) {
		t.Parallel()

		reqMD := renamedMaskDescriptor(t)
		resFD, maskFD := reqMD.Fields().ByName("resource"), reqMD.Fields().ByName("changes")
		fmOpts := []fieldmask.Option{fieldmask.WithMaskFieldName("changes")}
		run := chain(t, nil, fmOpts)
		req := func(paths ...string) *dynamicpb.Message {
			m := dynamicpb.NewMessage(reqMD)
			res := &pb.Resource{Name: "n", TenantId: "other", Description: "d"}
			m.Set(resFD, protoreflect.ValueOfMessage(dynamicResource(t, resFD.Message(), res)))
			m.Set(maskFD, protoreflect.ValueOfMessage((&fieldmaskpb.FieldMask{Paths: paths}).ProtoReflect()))
			return m
		}

		seen, err := run("/x.v1.X/UpdateResource", req("tenant_id"))
		require.NoError(t, err)
		require.NotNil(t, seen)

		seen, err = run("/x.v1.X/UpdateResource", req("description"))
		require.NoError(t, err)
		got := seen.(*dynamicpb.Message).Get(resFD).Message()
		require.False(t, got.Has(resFD.Message().Fields().ByName("tenant_id")), "unselected IMMUTABLE cleared")
	})
}

// renamedMaskDescriptor builds UpdateReq{fieldbehaviortest.v1.Resource
// resource = 1; google.protobuf.FieldMask changes = 2;}.
func renamedMaskDescriptor(tb testing.TB) protoreflect.MessageDescriptor {
	tb.Helper()

	resMD := (&pb.Resource{}).ProtoReflect().Descriptor()
	msg := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	fdp := &descriptorpb.FileDescriptorProto{
		Name:       new("fieldbehavior_renamed_mask_test.proto"),
		Package:    new("fieldbehaviorrenamedtest"),
		Syntax:     new("proto3"),
		Dependency: []string{resMD.ParentFile().Path(), "google/protobuf/field_mask.proto"},
		MessageType: []*descriptorpb.DescriptorProto{{Name: new("UpdateReq"), Field: []*descriptorpb.FieldDescriptorProto{
			{Name: new("resource"), Number: new(int32(1)), Type: &msg, Label: &opt, TypeName: new("." + string(resMD.FullName()))},
			{Name: new("changes"), Number: new(int32(2)), Type: &msg, Label: &opt, TypeName: new(".google.protobuf.FieldMask")},
		}}},
	}
	fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	require.NoError(tb, err)
	return fd.Messages().ByName("UpdateReq")
}

// dynamicResource returns res as a message of md (the generated type held by
// a dynamic request).
func dynamicResource(tb testing.TB, md protoreflect.MessageDescriptor, res *pb.Resource) protoreflect.Message {
	tb.Helper()

	if md == res.ProtoReflect().Descriptor() {
		return res.ProtoReflect()
	}
	m := dynamicpb.NewMessage(md)
	b, err := proto.Marshal(res)
	require.NoError(tb, err)
	require.NoError(tb, proto.Unmarshal(b, m))
	return m
}

// formatDescriptor builds
//
//	message FmtRes {
//	  string ref = 1 [(google.api.field_info).format = UUID4];
//	  string note = 2;
//	  string id = 3 [(google.api.field_behavior) = IDENTIFIER, (google.api.field_info).format = UUID4];
//	}
//	message UpdateFmtRequest { FmtRes resource = 1; google.protobuf.FieldMask update_mask = 2; }
func formatDescriptor(tb testing.TB) protoreflect.MessageDescriptor {
	tb.Helper()

	uuid := &descriptorpb.FieldOptions{}
	proto.SetExtension(uuid, annotations.E_FieldInfo, &annotations.FieldInfo{Format: annotations.FieldInfo_UUID4})
	identifier := proto.CloneOf(uuid)
	proto.SetExtension(identifier, annotations.E_FieldBehavior, []annotations.FieldBehavior{annotations.FieldBehavior_IDENTIFIER})
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	msg := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	fdp := &descriptorpb.FileDescriptorProto{
		Name:       new("fieldbehavior_format_test.proto"),
		Package:    new("fieldbehaviorformattest"),
		Syntax:     new("proto3"),
		Dependency: []string{"google/protobuf/field_mask.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: new("FmtRes"), Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("ref"), Number: new(int32(1)), Type: &str, Label: &opt, Options: uuid},
				{Name: new("note"), Number: new(int32(2)), Type: &str, Label: &opt},
				{Name: new("id"), Number: new(int32(3)), Type: &str, Label: &opt, Options: identifier},
			}},
			{Name: new("UpdateFmtRequest"), Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("resource"), Number: new(int32(1)), Type: &msg, Label: &opt, TypeName: new(".fieldbehaviorformattest.FmtRes")},
				{Name: new("update_mask"), Number: new(int32(2)), Type: &msg, Label: &opt, TypeName: new(".google.protobuf.FieldMask")},
			}},
		},
	}
	fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	require.NoError(tb, err)
	return fd.Messages().ByName("UpdateFmtRequest")
}

// TestUpdate_FormatsOfUsedFields pins where an Update checks AIP-202 formats:
// on the fields left once the update_mask is applied, which are the fields
// the update uses and the IDENTIFIER addressing the resource, or the request
// as sent for an update that changes nothing. The mask is applied the same
// with or without the fieldmask interceptor.
func TestUpdate_FormatsOfUsedFields(t *testing.T) {
	t.Parallel()

	reqMD := formatDescriptor(t)
	resFD, maskFD := reqMD.Fields().ByName("resource"), reqMD.Fields().ByName("update_mask")
	resMD := resFD.Message()
	const goodID = "0b6f7d1e-3c4a-4b5e-9f6a-7d8e9f0a1b2c"
	req := func(id string, paths []string) *dynamicpb.Message {
		res := dynamicpb.NewMessage(resMD)
		res.Set(resMD.Fields().ByName("ref"), protoreflect.ValueOfString("not-a-uuid"))
		res.Set(resMD.Fields().ByName("note"), protoreflect.ValueOfString("n"))
		res.Set(resMD.Fields().ByName("id"), protoreflect.ValueOfString(id))
		m := dynamicpb.NewMessage(reqMD)
		m.Set(resFD, protoreflect.ValueOfMessage(res))
		if paths != nil {
			m.Set(maskFD, protoreflect.ValueOfMessage((&fieldmaskpb.FieldMask{Paths: paths}).ProtoReflect()))
		}
		return m
	}

	tests := []struct {
		name    string
		id      string
		paths   []string
		want    []string // violation paths; nil for an accepted request
		invalid bool     // refused without a field violation (a path error)
	}{
		{name: "unmasked bad value", id: goodID, paths: []string{"note"}},
		{name: "masked bad value", id: goodID, paths: []string{"ref"}, want: []string{"resource.ref"}},
		{name: "bad identifier", id: "not-a-uuid", paths: []string{"note"}, want: []string{"resource.id"}},
		{name: "empty mask", id: goodID, paths: []string{}},
		{name: "empty mask, bad identifier", id: "not-a-uuid", paths: []string{}, want: []string{"resource.id"}},
		{name: "wildcard", id: goodID, paths: []string{"*"}, want: []string{"resource.ref"}},
		{name: "omitted mask", id: goodID, want: []string{"resource.ref"}},
		{name: "unknown path", id: goodID, paths: []string{"note", "nope"}, invalid: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var fmOpts []fieldmask.Option
			runs := map[string]func(string, any) (any, error){
				"alone": func(method string, r any) (any, error) {
					seen, _, err := runUnary(t, newUnary(t, fieldbehavior.WithUpdateNormalizer(fieldmask.UpdateNormalizer(fmOpts...))), method, r, &pb.Resource{}, nil)
					return seen, err
				},
				"with fieldmask": chain(t, nil, fmOpts),
			}
			for name, run := range runs {
				seen, err := run("/x.v1.X/UpdateFmt", req(tc.id, tc.paths))
				if tc.want == nil && !tc.invalid {
					require.NoError(t, err, name)
					got := seen.(*dynamicpb.Message).Get(resFD).Message()
					require.False(t, got.Has(resMD.Fields().ByName("ref")), "%s: the unmasked field is dropped", name)
					if len(tc.paths) == 0 {
						require.False(t, got.Has(resMD.Fields().ByName("note")), name)
						require.Equal(t, tc.id, got.Get(resMD.Fields().ByName("id")).String(), name)
						continue
					}
					require.Equal(t, "n", got.Get(resMD.Fields().ByName("note")).String(), name)
					continue
				}
				require.Equal(t, codes.InvalidArgument, status.Code(err), "%s: %v", name, err)
				require.Nil(t, seen, name)
				if tc.want != nil {
					require.Equal(t, tc.want, violationPaths(t, err), name)
				}
			}
		})
	}
}

// collectionDescriptor builds
//
//	message Item { string kind = 1 [(google.api.field_behavior) = REQUIRED, IMMUTABLE]; int32 qty = 2; }
//	message Order { string id = 1 [IDENTIFIER]; repeated Item items = 2; map<string, Item> by_code = 3; string note = 4; }
//	message UpdateOrderRequest { Order order = 1; google.protobuf.FieldMask update_mask = 2; }
func collectionDescriptor(tb testing.TB) protoreflect.MessageDescriptor {
	tb.Helper()

	behaviors := func(b ...annotations.FieldBehavior) *descriptorpb.FieldOptions {
		o := &descriptorpb.FieldOptions{}
		proto.SetExtension(o, annotations.E_FieldBehavior, b)
		return o
	}
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	i32 := descriptorpb.FieldDescriptorProto_TYPE_INT32
	msg := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	rep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	fdp := &descriptorpb.FileDescriptorProto{
		Name:       new("fieldbehavior_collection_test.proto"),
		Package:    new("fieldbehaviorcollectiontest"),
		Syntax:     new("proto3"),
		Dependency: []string{"google/protobuf/field_mask.proto", "google/api/field_behavior.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: new("Item"), Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("kind"), Number: new(int32(1)), Type: &str, Label: &opt,
					Options: behaviors(annotations.FieldBehavior_REQUIRED, annotations.FieldBehavior_IMMUTABLE)},
				{Name: new("qty"), Number: new(int32(2)), Type: &i32, Label: &opt},
			}},
			{Name: new("Order"), Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("id"), Number: new(int32(1)), Type: &str, Label: &opt, Options: behaviors(annotations.FieldBehavior_IDENTIFIER)},
				{Name: new("items"), Number: new(int32(2)), Type: &msg, Label: &rep, TypeName: new(".fieldbehaviorcollectiontest.Item")},
				{Name: new("by_code"), Number: new(int32(3)), Type: &msg, Label: &rep, TypeName: new(".fieldbehaviorcollectiontest.Order.ByCodeEntry")},
				{Name: new("note"), Number: new(int32(4)), Type: &str, Label: &opt},
			}, NestedType: []*descriptorpb.DescriptorProto{{
				Name:    new("ByCodeEntry"),
				Options: &descriptorpb.MessageOptions{MapEntry: new(true)},
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: new("key"), Number: new(int32(1)), Type: &str, Label: &opt},
					{Name: new("value"), Number: new(int32(2)), Type: &msg, Label: &opt, TypeName: new(".fieldbehaviorcollectiontest.Item")},
				},
			}}},
			{Name: new("UpdateOrderRequest"), Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("order"), Number: new(int32(1)), Type: &msg, Label: &opt, TypeName: new(".fieldbehaviorcollectiontest.Order")},
				{Name: new("update_mask"), Number: new(int32(2)), Type: &msg, Label: &opt, TypeName: new(".google.protobuf.FieldMask")},
			}},
		},
	}
	fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	require.NoError(tb, err)
	return fd.Messages().ByName("UpdateOrderRequest")
}

// TestUpdate_CollectionReplacement verifies the v0.1.5 contract.
// Protected collection replacements are deferred; numeric list indices remain invalid.
func TestUpdate_CollectionReplacement(t *testing.T) {
	t.Parallel()

	reqMD := collectionDescriptor(t)
	orderFD, maskFD := reqMD.Fields().ByName("order"), reqMD.Fields().ByName("update_mask")
	orderMD := orderFD.Message()
	itemsFD, byCodeFD := orderMD.Fields().ByName("items"), orderMD.Fields().ByName("by_code")
	itemMD := itemsFD.Message()
	kindFD := itemMD.Fields().ByName("kind")
	item := func() protoreflect.Value {
		it := dynamicpb.NewMessage(itemMD)
		it.Set(kindFD, protoreflect.ValueOfString("physical"))
		it.Set(itemMD.Fields().ByName("qty"), protoreflect.ValueOfInt32(2))
		return protoreflect.ValueOfMessage(it)
	}
	req := func(paths ...string) *dynamicpb.Message {
		order := dynamicpb.NewMessage(orderMD)
		order.Set(orderMD.Fields().ByName("id"), protoreflect.ValueOfString("o-1"))
		order.Mutable(itemsFD).List().Append(item())
		order.Mutable(byCodeFD).Map().Set(protoreflect.ValueOfString("a").MapKey(), item())
		m := dynamicpb.NewMessage(reqMD)
		m.Set(orderFD, protoreflect.ValueOfMessage(order))
		m.Set(maskFD, protoreflect.ValueOfMessage((&fieldmaskpb.FieldMask{Paths: paths}).ProtoReflect()))
		return m
	}

	for name, run := range map[string]func(string, any) (any, error){
		"alone": func(method string, r any) (any, error) {
			seen, _, err := runUnary(t, newUnary(t), method, r, &pb.Resource{}, nil)
			return seen, err
		},
		"with fieldmask": chain(t, nil, nil),
	} {
		for _, paths := range [][]string{{"items"}, {"by_code"}, {"by_code.a"}} {
			seen, err := run("/x.v1.X/UpdateOrder", req(paths...))
			require.NoError(t, err, "%s %q", name, paths)
			order := seen.(*dynamicpb.Message).Get(orderFD).Message()
			if paths[0] == "items" {
				require.Equal(t, "physical", order.Get(itemsFD).List().Get(0).Message().Get(kindFD).String(), "%s %q", name, paths)
			} else {
				got := order.Get(byCodeFD).Map().Get(protoreflect.ValueOfString("a").MapKey())
				require.Equal(t, "physical", got.Message().Get(kindFD).String(), "%s %q", name, paths)
			}
		}

		for _, paths := range [][]string{{"items.0.kind"}} {
			seen, err := run("/x.v1.X/UpdateOrder", req(paths...))
			require.Equal(t, codes.InvalidArgument, status.Code(err), "%s %q: %v", name, paths, err)
			require.Nil(t, seen, "%s %q", name, paths)
		}
	}
}

// wrapperDescriptor builds
//
//	message UpdateWrappedRequest {
//	  fieldbehaviortest.v1.Resource resource = 1;
//	  google.protobuf.FieldMask update_mask = 2;
//	  string revision = 3 [(google.api.field_behavior) = OUTPUT_ONLY];
//	  string parent = 4 [(google.api.field_behavior) = IMMUTABLE];
//	  bool validate_only = 5;
//	}
func wrapperDescriptor(tb testing.TB) protoreflect.MessageDescriptor {
	tb.Helper()

	behavior := func(b annotations.FieldBehavior) *descriptorpb.FieldOptions {
		o := &descriptorpb.FieldOptions{}
		proto.SetExtension(o, annotations.E_FieldBehavior, []annotations.FieldBehavior{b})
		return o
	}
	resMD := (&pb.Resource{}).ProtoReflect().Descriptor()
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	boolean := descriptorpb.FieldDescriptorProto_TYPE_BOOL
	msg := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	fdp := &descriptorpb.FileDescriptorProto{
		Name:       new("fieldbehavior_wrapper_test.proto"),
		Package:    new("fieldbehaviorwrappertest"),
		Syntax:     new("proto3"),
		Dependency: []string{resMD.ParentFile().Path(), "google/protobuf/field_mask.proto", "google/api/field_behavior.proto"},
		MessageType: []*descriptorpb.DescriptorProto{{Name: new("UpdateWrappedRequest"), Field: []*descriptorpb.FieldDescriptorProto{
			{Name: new("resource"), Number: new(int32(1)), Type: &msg, Label: &opt, TypeName: new("." + string(resMD.FullName()))},
			{Name: new("update_mask"), Number: new(int32(2)), Type: &msg, Label: &opt, TypeName: new(".google.protobuf.FieldMask")},
			{Name: new("revision"), Number: new(int32(3)), Type: &str, Label: &opt, Options: behavior(annotations.FieldBehavior_OUTPUT_ONLY)},
			{Name: new("parent"), Number: new(int32(4)), Type: &str, Label: &opt, Options: behavior(annotations.FieldBehavior_IMMUTABLE)},
			{Name: new("validate_only"), Number: new(int32(5)), Type: &boolean, Label: &opt},
		}}},
	}
	fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	require.NoError(tb, err)
	return fd.Messages().ByName("UpdateWrappedRequest")
}

// TestUpdate_RequestFieldsOutsideResource verifies the v0.1.5 contract.
// Outside-resource fields retain control values and protected values while clearing OUTPUT_ONLY.
func TestUpdate_RequestFieldsOutsideResource(t *testing.T) {
	t.Parallel()

	reqMD := wrapperDescriptor(t)
	fields := reqMD.Fields()
	resFD, maskFD := fields.ByName("resource"), fields.ByName("update_mask")
	revisionFD, parentFD, validateFD := fields.ByName("revision"), fields.ByName("parent"), fields.ByName("validate_only")
	req := func(parent string, paths ...string) *dynamicpb.Message {
		m := dynamicpb.NewMessage(reqMD)
		res := &pb.Resource{Id: "id-1", Name: "n", Description: "d"}
		m.Set(resFD, protoreflect.ValueOfMessage(dynamicResource(t, resFD.Message(), res)))
		m.Set(maskFD, protoreflect.ValueOfMessage((&fieldmaskpb.FieldMask{Paths: paths}).ProtoReflect()))
		m.Set(revisionFD, protoreflect.ValueOfString("client-made"))
		m.Set(validateFD, protoreflect.ValueOfBool(true))
		if parent != "" {
			m.Set(parentFD, protoreflect.ValueOfString(parent))
		}
		return m
	}

	for name, run := range map[string]func(string, any) (any, error){
		"alone": func(method string, r any) (any, error) {
			seen, _, err := runUnary(t, newUnary(t), method, r, &pb.Resource{}, nil)
			return seen, err
		},
		"with fieldmask": chain(t, nil, nil),
	} {
		for _, paths := range [][]string{{"description"}, {"*"}, {}} {
			seen, err := run("/x.v1.X/UpdateWrapped", req("", paths...))
			require.NoError(t, err, "%s %q", name, paths)
			got := seen.(*dynamicpb.Message)
			require.False(t, got.Has(revisionFD), "%s %q: OUTPUT_ONLY beside the resource cleared", name, paths)
			require.True(t, got.Get(validateFD).Bool(), "%s %q: plain field kept", name, paths)
			require.True(t, got.Has(resFD), "%s %q: the resource stays in the request", name, paths)
			res := got.Get(resFD).Message().Interface().(*pb.Resource)
			if len(paths) > 0 {
				require.Equal(t, "d", res.GetDescription(), "%s %q", name, paths)
			}
			require.Equal(t, "id-1", res.GetId(), "%s %q", name, paths)

			seen, err = run("/x.v1.X/UpdateWrapped", req("other", paths...))
			require.NoError(t, err)
			require.Equal(t, "other", seen.(*dynamicpb.Message).Get(parentFD).String())
		}
	}
}

// TestUpdate_ResourceNotHeldByRequest pins that an extractor handing out a
// resource the request does not hold fails the call: the mask was applied to
// a message the handler never sees.
func TestUpdate_ResourceNotHeldByRequest(t *testing.T) {
	t.Parallel()

	detached := updatemask.WithExtractor(func(_ context.Context, req proto.Message) (*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
		r := req.(*pb.UpdateResourceRequest)
		return r.GetUpdateMask(), proto.CloneOf(r.GetResource()), func(m *fieldmaskpb.FieldMask) { r.UpdateMask = m }, true
	})
	uni := newUnary(t, fieldbehavior.WithUpdateNormalizer(updatemask.New(detached)))
	req := &pb.UpdateResourceRequest{Resource: &pb.Resource{Id: "id-1", Description: "d"}, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"description"}}}
	seen, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.Equal(t, codes.Internal, status.Code(err), "%v", err)
	require.Nil(t, seen)
}

// TestUpdate_NoopPreservesProtectedValues verifies the v0.1.5 contract.
// No-op preparation clears unselected immutable input and keeps routing identities.
func TestUpdate_NoopPreservesProtectedValues(t *testing.T) {
	t.Parallel()

	for name, run := range map[string]func(string, any) (any, error){
		"alone": func(method string, r any) (any, error) {
			seen, _, err := runUnary(t, newUnary(t), method, r, &pb.Resource{}, nil)
			return seen, err
		},
		"with fieldmask": chain(t, nil, nil),
	} {
		for _, paths := range [][]string{{}, {"create_time"}, {"create_time", "profile.updated_at"}} {
			req := &pb.UpdateResourceRequest{
				Resource: &pb.Resource{
					Id: "id-1", Name: "n", TenantId: "other", CreateTime: "client-made",
					Profile: &pb.Profile{DisplayName: "p", UpdatedAt: "client-made"},
				},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: paths},
			}
			seen, err := run("/x.v1.X/UpdateResource", req)
			require.NoError(t, err, "%s %q", name, paths)
			got := seen.(*pb.UpdateResourceRequest)
			require.NotNil(t, got.GetUpdateMask(), "%s %q", name, paths)
			require.Empty(t, got.GetUpdateMask().GetPaths(), "%s %q", name, paths)
			require.Equal(t, "id-1", got.GetResource().GetId())
			require.Empty(t, got.GetResource().GetTenantId())
		}
	}
}

// TestUpdate_NoopKeepsPolicyIdentity pins that an update that changes
// nothing keeps the fields that address the resource under the configured
// update policy, as any other mask does: the IDENTIFIER fields of a custom
// resolver and the identity fields of update.WithIdentityFields.
func TestUpdate_NoopKeepsPolicyIdentity(t *testing.T) {
	t.Parallel()

	resolver := update.WithResolver(pbfieldbehavior.ResolverFunc(func(fd protoreflect.FieldDescriptor) []annotations.FieldBehavior {
		if fd.Name() == "name" {
			return []annotations.FieldBehavior{annotations.FieldBehavior_IDENTIFIER}
		}
		return pbfieldbehavior.AnnotationResolver{}.Resolve(fd)
	}))
	outputOnlyIdentifier := update.WithResolver(pbfieldbehavior.ResolverFunc(func(fd protoreflect.FieldDescriptor) []annotations.FieldBehavior {
		if fd.Name() == "name" {
			return []annotations.FieldBehavior{annotations.FieldBehavior_IDENTIFIER, annotations.FieldBehavior_OUTPUT_ONLY}
		}
		return pbfieldbehavior.AnnotationResolver{}.Resolve(fd)
	}))
	policies := map[string]struct {
		opt  update.Option
		want *pb.Resource
	}{
		"resolver":        {opt: resolver, want: &pb.Resource{Id: "id-1", Name: "n"}},
		"identity fields": {opt: update.WithIdentityFields("slug"), want: &pb.Resource{Id: "id-1", Slug: "s"}},
		// An identifier the resolver also marks OUTPUT_ONLY is not input.
		"output-only identifier": {opt: outputOnlyIdentifier, want: &pb.Resource{Id: "id-1"}},
	}

	for policy, tc := range policies {
		fmOpts := []fieldmask.Option{fieldmask.WithUpdateOptions(tc.opt)}
		for name, run := range map[string]func(string, any) (any, error){
			"alone": func(method string, r any) (any, error) {
				uni := newUnary(t, fieldbehavior.WithUpdateNormalizer(fieldmask.UpdateNormalizer(fmOpts...)))
				seen, _, err := runUnary(t, uni, method, r, &pb.Resource{}, nil)
				return seen, err
			},
			"with fieldmask": chain(t, nil, fmOpts),
		} {
			for _, paths := range [][]string{{}, {"create_time"}} {
				req := &pb.UpdateResourceRequest{
					Resource:   &pb.Resource{Id: "id-1", Name: "n", Slug: "s", Description: "d", CreateTime: "client-made"},
					UpdateMask: &fieldmaskpb.FieldMask{Paths: paths},
				}
				seen, err := run("/x.v1.X/UpdateResource", req)
				require.NoError(t, err, "%s %s %q", policy, name, paths)
				got := seen.(*pb.UpdateResourceRequest).GetResource()
				want := proto.CloneOf(tc.want)
				require.True(t, proto.Equal(want, got), "%s %s %q: got %v, want %v", policy, name, paths, got, want)
			}
		}
	}
}

// Exercise completion through the same extractor and interceptor combinations
// whose unselected immutable assertions changed for Prepare's v0.1.8 contract.
func TestUpdate_SelectedImmutableCompletion(t *testing.T) {
	t.Parallel()

	const method = "/x.v1.X/UpdateResource"
	nested := fieldmask.WithUpdateExtractor(method, extract.NewUpdateFunc(
		func(r *pb.NestedUpdateResourceRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetUpdateMask() },
		func(r *pb.NestedUpdateResourceRequest) *pb.Resource { return r.GetResource() },
		func(r *pb.NestedUpdateResourceRequest, m *fieldmaskpb.FieldMask) { r.Options.UpdateMask = m },
	))
	for _, config := range []struct {
		name        string
		opts        []fieldmask.Option
		makeRequest func(*pb.Resource, *fieldmaskpb.FieldMask) proto.Message
	}{
		{name: "default"},
		{name: "excluded", opts: []fieldmask.Option{fieldmask.WithIgnoreMethods(method)}},
		{name: "skipped", opts: []fieldmask.Option{fieldmask.WithMethodKind(method, fieldmask.KindSkip)}},
		{name: "policy identity", opts: []fieldmask.Option{fieldmask.WithUpdateOptions(update.WithIdentityFields("slug"))}},
		{name: "nested", opts: []fieldmask.Option{nested}, makeRequest: func(r *pb.Resource, m *fieldmaskpb.FieldMask) proto.Message {
			return &pb.NestedUpdateResourceRequest{Resource: r, Options: &pb.NestedUpdateResourceRequest_Options{UpdateMask: m}}
		}},
		{name: "renamed", opts: []fieldmask.Option{fieldmask.WithMaskFieldName("changes")}, makeRequest: func(r *pb.Resource, m *fieldmaskpb.FieldMask) proto.Message {
			md := renamedMaskDescriptor(t)
			req := dynamicpb.NewMessage(md)
			req.Set(md.Fields().ByName("resource"), protoreflect.ValueOfMessage(r.ProtoReflect()))
			req.Set(md.Fields().ByName("changes"), protoreflect.ValueOfMessage(m.ProtoReflect()))
			return req
		}},
	} {
		t.Run(config.name, func(t *testing.T) {
			for _, withMask := range []bool{false, true} {
				for _, changed := range []bool{false, true} {
					for _, field := range []string{"tenant_id", "slug"} {
						// Under this policy slug is a routing identity, not immutable.
						if config.name == "policy identity" && field == "slug" {
							continue
						}
						resource := &pb.Resource{Id: "id-1", TenantId: "sent", Slug: "sent"}
						stored := proto.CloneOf(resource)
						fd := resource.ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name(field))
						if changed {
							stored.ProtoReflect().Set(fd, protoreflect.ValueOfString("stored"))
						}
						mask := &fieldmaskpb.FieldMask{Paths: []string{field}}
						var req proto.Message = &pb.UpdateResourceRequest{Resource: resource, UpdateMask: mask}
						if config.makeRequest != nil {
							req = config.makeRequest(resource, mask)
						}
						fb := newUnary(t, fieldbehavior.WithUpdateNormalizer(fieldmask.UpdateNormalizer(config.opts...)))
						fm := fieldmask.ServerInterceptor(config.opts...).ServerUnaryInterceptor()
						info := &grpc.UnaryServerInfo{FullMethod: method}
						called := false
						handler := func(ctx context.Context, _ any) (any, error) {
							called = true
							plan, ok := update.PlanFromContext(ctx, resource)
							require.True(t, ok)
							require.Equal(t, "sent", resource.ProtoReflect().Get(fd).String())
							resource.ProtoReflect().Clear(fd)
							effective, err := plan.Complete(stored)
							if changed {
								var behavior *update.BehaviorError
								require.ErrorAs(t, err, &behavior)
								require.Equal(t, field, behavior.Violations[0].Path)
							} else {
								require.NoError(t, err)
								require.Empty(t, effective)
							}
							return &pb.Resource{}, nil
						}
						_, err := fb(t.Context(), req, info, func(ctx context.Context, r any) (any, error) {
							if withMask {
								return fm(ctx, r, info, handler)
							}
							return handler(ctx, r)
						})
						require.NoError(t, err)
						require.True(t, called)
					}
				}
			}
		})
	}
}
