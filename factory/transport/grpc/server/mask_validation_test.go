// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/fieldmask/update"
	"github.com/altessa-s/proto-gen-go/io/altessa/badrequest/v1"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldbehavior"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/updatemask"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	aipbehavior "github.com/altessa-s/go-aip/fieldbehavior"
	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	pb "github.com/altessa-s/go-atlas/proto/gen/maskvalidationtest/v1"
	bufhelpers "github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator/buf"
)

func TestMaskAwareValidationFactoryChain(t *testing.T) {
	t.Parallel()
	const service = "maskvalidationtest.v1.SettingsService"
	const scopedMethod = "/" + service + "/UpdateScoped"
	const atomicMethod = "/" + service + "/UpdateAtomic"
	const requiredNestedMethod = "/" + service + "/UpdateRequiredNested"
	const policyMethod = "/" + service + "/UpdatePolicy"
	const legacyUnlocatedMethod = "/" + service + "/UpdateSynonymGroup"
	const brokenMethod = "/" + service + "/UpdateBroken"
	const registeredMethod = "/" + service + "/UpdateRegistered"
	const unlocatedMethod = "/" + service + "/UpdateUnlocated"
	spec, err := update.NewSpec(new(pb.Settings),
		update.WithAllowedPaths("general", "general.time_zone", "general.retention_days"))
	require.NoError(t, err)
	scopedSpec, err := update.NewSpec(new(pb.Settings))
	require.NoError(t, err)
	atomicSpec, err := update.NewSpec(new(pb.Settings),
		update.WithAtomicMessage(func(md protoreflect.MessageDescriptor) bool {
			return md.FullName() == "maskvalidationtest.v1.General" || update.DefaultAtomicMessage(md)
		}))
	require.NoError(t, err)
	requiredNestedSpec, err := update.NewSpec(new(pb.Settings), update.WithResolver(aipbehavior.ResolverFunc(
		func(fd protoreflect.FieldDescriptor) []annotations.FieldBehavior {
			if fd.FullName() == "maskvalidationtest.v1.Route.nested" {
				return []annotations.FieldBehavior{annotations.FieldBehavior_REQUIRED}
			}
			return (aipbehavior.AnnotationResolver{}).Resolve(fd)
		})))
	require.NoError(t, err)
	policySpec, err := update.NewSpec(new(pb.Settings),
		update.WithIdentityFields("routing_key"), update.WithResolver(settingsBehavior{}))
	require.NoError(t, err)
	run := func(ctx context.Context, raw any) (any, error) {
		req := raw.(*pb.UpdateSettingsRequest)
		sel, ok := updatemask.SelectionFromContext(ctx, req)
		grpcMethod, _ := grpc.Method(ctx)
		if grpcMethod == legacyUnlocatedMethod {
			if ok {
				return nil, status.Error(codes.Internal, "unregistered extractor miss published a selection")
			}
			index := len(req.GetUpdateMask().GetPaths()) - 1
			return nil, indexedTestMaskError(index)
		}
		if grpcMethod == unlocatedMethod && ok {
			return nil, status.Error(codes.Internal, "extractor miss published a selection")
		}
		if !ok {
			return req, nil
		}
		if sel.Spec != spec && sel.Spec != scopedSpec && sel.Spec != policySpec && sel.Spec != atomicSpec && sel.Spec != requiredNestedSpec {
			return nil, status.Error(codes.Internal, "unexpected spec")
		}
		if sel.Resource != req.Settings || !proto.Equal(sel.OriginalMask(), req.UpdateMask) {
			return nil, status.Error(codes.Internal, "registered selection lost the request or original mask")
		}
		if sel.Err != nil {
			return nil, status.Error(codes.InvalidArgument, sel.Err.Error())
		}
		if _, err := sel.Spec.ValidateResolved(sel.Resource, sel.Mask); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return req, nil
	}
	method := func(name string) grpc.MethodDesc {
		return grpc.MethodDesc{MethodName: name, Handler: func(srv any, ctx context.Context, dec func(any) error,
			ic grpc.UnaryServerInterceptor) (any, error) {
			req := new(pb.UpdateSettingsRequest)
			if err := dec(req); err != nil {
				return nil, err
			}
			return ic(ctx, req, &grpc.UnaryServerInfo{Server: srv, FullMethod: "/" + service + "/" + name}, run)
		}}
	}
	var extractions atomic.Int32
	extract := updatemask.DefaultExtractor()
	b := New(&grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		FieldMask: &grpcconfig.FieldMaskInterceptor{BaseInterceptor: enabled},
	}}).UseReasonCode(func(rule, _ string) string { return rule }).
		UseFieldMaskOptions(
			fieldmask.WithUpdateSpec(registeredMethod, spec),
			fieldmask.WithUpdateSpec(atomicMethod, atomicSpec),
			fieldmask.WithUpdateSpec(requiredNestedMethod, requiredNestedSpec),
			fieldmask.WithUpdateSpec(scopedMethod, scopedSpec),
			fieldmask.WithUpdateSpec(unlocatedMethod, scopedSpec),
			fieldmask.WithUpdateSpec(policyMethod, policySpec),
			fieldmask.WithUpdateSpec(brokenMethod, scopedSpec),
			fieldmask.WithUpdateExtractor(brokenMethod, func(context.Context, proto.Message) (
				*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
				return nil, &pb.Settings{}, func(*fieldmaskpb.FieldMask) {}, true
			}),
			fieldmask.WithUpdateExtractor(legacyUnlocatedMethod, func(context.Context, proto.Message) (
				*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
				return nil, nil, nil, false
			}),
			fieldmask.WithUpdateExtractor(registeredMethod, func(ctx context.Context, req proto.Message) (
				*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
				extractions.Add(1)
				return extract(ctx, req)
			}),
			fieldmask.WithUpdateExtractor(unlocatedMethod, func(_ context.Context, req proto.Message) (
				*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
				if req.(*pb.UpdateSettingsRequest).GetCaller() == "nil" {
					return nil, (*pb.Settings)(nil), nil, true
				}
				return nil, nil, nil, false
			}),
		).WithInterceptors()
	conn := serveService(t, b, &grpc.ServiceDesc{
		ServiceName: service,
		HandlerType: (*any)(nil),
		Methods:     []grpc.MethodDesc{method("UpdateSettings"), method("UpdateAtomic"), method("UpdateRequiredNested"), method("UpdateScoped"), method("CreateSettings"), method("UpdateRegistered"), method("UpdateUnlocated"), method("UpdatePolicy"), method("UpdateSynonymGroup"), method("UpdateBroken")},
		Streams: []grpc.StreamDesc{{StreamName: "UpdateStream", ClientStreams: true, Handler: func(_ any, s grpc.ServerStream) error {
			req := new(pb.UpdateSettingsRequest)
			if err := s.RecvMsg(req); err != nil {
				return err
			}
			return s.SendMsg(req)
		}}},
	})

	tests := []struct {
		name, method string
		mask         []string
		req          *pb.UpdateSettingsRequest
		path, rule   string
	}{
		{name: "leaf without required sibling", mask: []string{"general.retentionDays"},
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{RetentionDays: 7}}}},
		{name: "explicit clear", mask: []string{"general.timeZone"},
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{}}},
			path: "settings.general.time_zone", rule: "required"},
		{name: "absent parent", mask: []string{"general"},
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{}}, path: "settings.general.time_zone", rule: "required"},
		{name: "wildcard", mask: []string{"*"},
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{}}, path: "settings.general.time_zone", rule: "required"},
		{name: "second list element", mask: []string{"notifications.routes"},
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{Notifications: &pb.Notifications{
				Routes: []*pb.Route{{Kind: pb.Route_KIND_EMAIL}, {}}}}}, path: "settings.notifications.routes[1].kind", rule: "required"},
		{name: "nested list element", mask: []string{"notifications.routes"},
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{Notifications: &pb.Notifications{
				Routes: []*pb.Route{{Kind: pb.Route_KIND_EMAIL}, {Kind: pb.Route_KIND_EMAIL, Nested: &pb.General{}}},
			}}}, path: "settings.notifications.routes[1].nested.time_zone", rule: "required"},
		{name: "list element path validates original siblings", mask: []string{"notifications.routes.*.nested.retentionDays"},
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{Notifications: &pb.Notifications{
				Routes: []*pb.Route{{Kind: pb.Route_KIND_EMAIL, Nested: &pb.General{TimeZone: "UTC", RetentionDays: 6}}},
			}}}},
		{name: "list element path checks required sibling", mask: []string{"notifications.routes.*.nested.retentionDays"},
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{Notifications: &pb.Notifications{
				Routes: []*pb.Route{{Nested: &pb.General{TimeZone: "UTC", RetentionDays: 6}}},
			}}}, path: "settings.notifications.routes[0].kind", rule: "required"},
		{name: "absent optional nested message in selected map entry", mask: []string{"delegates.bob"},
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{Delegates: map[string]*pb.Route{"bob": {Kind: pb.Route_KIND_EMAIL}}}}},
		{name: "explicit map leaf under absent optional message retains clear intent", mask: []string{"delegates.bob.nested.timeZone"},
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{Delegates: map[string]*pb.Route{"bob": {Kind: pb.Route_KIND_EMAIL}}}},
			path: `settings.delegates["bob"].nested.time_zone`, rule: "required"},
		{name: "absent optional nested message in selected list element", mask: []string{"notifications.routes"},
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{Notifications: &pb.Notifications{Routes: []*pb.Route{{Kind: pb.Route_KIND_EMAIL}}}}}},
		{name: "wildcard and exact map scopes are combined", mask: []string{"delegates.*.nested.retention_days", "delegates.bob.kind"},
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{Delegates: map[string]*pb.Route{
				"bob":   {Kind: pb.Route_KIND_EMAIL, Nested: &pb.General{TimeZone: "UTC", RetentionDays: 3651}},
				"alice": {Nested: &pb.General{RetentionDays: 6}},
			}}}, path: `settings.delegates["bob"].nested.retention_days`, rule: "int32.lte"},
		{name: "invalid mask checks required inside atomic message", method: "UpdateAtomic", mask: []string{"bogus"},
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{}}},
			path: "settings.general.time_zone", rule: "required"},
		{name: "output timestamp outside allow-list is stripped", method: "UpdateRegistered", mask: []string{"general.time_zone"},
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{TimeZone: "UTC"}, UpdateTime: &timestamppb.Timestamp{Seconds: 1}}}},
		{name: "map required preserves quoted key", mask: []string{"delegates.`a.b`.kind"},
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{Delegates: map[string]*pb.Route{"a.b": {}}}},
			path: "settings.delegates[\"a.b\"].kind", rule: "required"},
		{name: "invalid mask checks whole body", mask: []string{"noSuchField"},
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{}}},
			path: "settings.general.time_zone", rule: "required"},
		{name: "extractor miss runs legacy validation", method: "UpdateUnlocated", mask: []string{"noSuchField"},
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{}}},
			path: "settings.general.time_zone", rule: "string.min_len"},
		{name: "omitted mask selects populated leaf",
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{RetentionDays: 9}}}},
		{name: "unselected invalid field", mask: []string{"general.timeZone"},
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{TimeZone: "UTC", RetentionDays: 3651}}}},
		{name: "registered skips invalid submitted sibling", method: "UpdateRegistered", mask: []string{"general.timeZone"},
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{TimeZone: "UTC", RetentionDays: 3651}}}},
		{name: "registered rejected path checks body first", method: "UpdateRegistered", mask: []string{"notifications"},
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{}}},
			path: "settings.general.time_zone", rule: "required"},
		{name: "selected rule retains protobuf path", mask: []string{"general.retentionDays"},
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{RetentionDays: 3651}}},
			path: "settings.general.retention_days", rule: "int32.lte"},
		{name: "partial selection skips message rule", mask: []string{"general.retentionDays"},
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{RetentionDays: 8}}}},
		{name: "whole parent checks field rule", mask: []string{"general"},
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{TimeZone: "UTC", RetentionDays: 7}}},
			path: "settings.general", rule: "general.field"},
		{name: "whole parent checks message rule", mask: []string{"general"},
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{TimeZone: "UTC", RetentionDays: 8}}},
			path: "settings.general", rule: "general.message"},
		{name: "identity", mask: []string{"general.retentionDays"},
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{Etag: "long-etag"}}, path: "settings.etag", rule: "string.max_len"},
		{name: "outside resource", mask: []string{},
			req: &pb.UpdateSettingsRequest{Caller: "invalid", Settings: &pb.Settings{}}, path: "caller", rule: "string.max_len"},
		{name: "non-update", method: "CreateSettings",
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{}}},
			path: "settings.general.time_zone", rule: "string.min_len"},
		{name: "legacy secret", method: "UpdateSettings",
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{ApiKey: "invalid"}}, path: "settings.api_key", rule: "string.max_len"},
		{name: "legacy immutable", method: "UpdateSettings",
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{Name: "invalid"}}, path: "settings.name", rule: "string.max_len"},
		{name: "legacy message CEL", method: "UpdateSettings", mask: []string{"general.time_zone", "general.retention_days"},
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{RetentionDays: 8, TimeZone: "UTC"}}},
			path: "settings.general", rule: "general.message"},
		{name: "scoped secret omitted mask",
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{ApiKey: "invalid"}}, path: "settings.api_key", rule: "string.max_len"},
		{name: "selected map value", mask: []string{"labels.good"},
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{Labels: map[string]string{"good": "invalid", "invalid": "invalid"}}},
			path: `settings.labels["good"]`, rule: "string.max_len"},
		{name: "selected map key", mask: []string{"labels.invalid"},
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{Labels: map[string]string{"good": "invalid", "invalid": "ok"}}},
			path: `settings.labels["invalid"]`, rule: "string.max_len"},
		{name: "custom identity is checked", method: "UpdatePolicy", mask: []string{"general.retention_days"},
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{RoutingKey: "invalid", Id: "also invalid"}},
			path: "settings.routing_key", rule: "string.max_len"},
		{name: "resolved mask respects resolver override", method: "UpdatePolicy", mask: []string{"general.time_zone"},
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{}}},
			path: "settings.general.time_zone", rule: "string.min_len"},
		{name: "unresolved mask respects resolver override", method: "UpdatePolicy", mask: []string{"bogus"},
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{}}},
			path: "settings.general.time_zone", rule: "string.min_len"},
		{name: "resolver can remove output-only", method: "UpdatePolicy", mask: []string{"server_note"},
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{ServerNote: "invalid"}},
			path: "settings.server_note", rule: "string.max_len"},
		{name: "resolver can add output-only", method: "UpdatePolicy", mask: []string{"bogus"},
			req:  &pb.UpdateSettingsRequest{Settings: &pb.Settings{FormatUuid: "invalid"}},
			path: "", rule: "handler mask refusal"},

		{name: "selected map ignores invalid neighbor", mask: []string{"labels.good"},
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{Labels: map[string]string{"good": "ok", "invalid": "invalid"}}}},

		{name: "unselected format", mask: []string{"general.time_zone"},
			req: &pb.UpdateSettingsRequest{Settings: &pb.Settings{FormatUuid: "invalid", General: &pb.General{TimeZone: "UTC"}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			method := tc.method
			if method == "" {
				method = "UpdateScoped"
			}
			if tc.mask != nil {
				tc.req.UpdateMask = &fieldmaskpb.FieldMask{Paths: tc.mask}
			}
			tc.req.OutsideRequired = new("valid")
			tc.req.Settings.Synonyms = "valid"
			reply := new(pb.UpdateSettingsRequest)
			err := conn.Invoke(t.Context(), "/"+service+"/"+method, tc.req, reply)
			if tc.rule == "" {
				require.NoError(t, err)
				if tc.req.GetSettings().GetUpdateTime() != nil {
					require.Nil(t, reply.GetSettings().GetUpdateTime())
				}
				if tc.req.GetSettings().GetGeneral().GetRetentionDays() == 3651 {
					require.Equal(t, int32(3651), reply.GetSettings().GetGeneral().GetRetentionDays())
				}
				return
			}
			if tc.rule == "handler mask refusal" {
				require.Equal(t, codes.InvalidArgument, status.Code(err))
				require.Contains(t, status.Convert(err).Message(), "updateMask.paths[0]")
				require.Empty(t, status.Convert(err).Details())
				return
			}
			checkMaskValidationViolation(t, err, tc.path, tc.rule)
		})
	}

	t.Run("wildcard and exact map scopes include selected AIP-202 formats", func(t *testing.T) {
		req := &pb.UpdateSettingsRequest{Settings: &pb.Settings{Delegates: map[string]*pb.Route{
			"bob": {Kind: pb.Route_KIND_EMAIL, FormatUuid: "invalid"},
		}}, OutsideRequired: new("valid"), UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"delegates.*.format_uuid", "delegates.bob.kind"}}}
		err := conn.Invoke(t.Context(), scopedMethod, req, new(pb.UpdateSettingsRequest))
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		detail := status.Convert(err).Details()[0].(*badrequestv1.BadRequest)
		require.Len(t, detail.GetFieldViolations(), 1)
		require.Contains(t, detail.GetFieldViolations()[0].GetFieldPath(), "bob")
	})

	t.Run("absent required nested messages in selected collection elements", func(t *testing.T) {
		for _, tc := range []struct {
			name, mask, prefix string
			settings           *pb.Settings
		}{
			{"map", "delegates.bob", `settings.delegates["bob"].nested`, &pb.Settings{Delegates: map[string]*pb.Route{"bob": {Kind: pb.Route_KIND_EMAIL}}}},
			{"list", "notifications.routes", "settings.notifications.routes[0].nested", &pb.Settings{Notifications: &pb.Notifications{Routes: []*pb.Route{{Kind: pb.Route_KIND_EMAIL}}}}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				req := &pb.UpdateSettingsRequest{Settings: tc.settings, OutsideRequired: new("valid"),
					UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{tc.mask}}}
				err := conn.Invoke(t.Context(), requiredNestedMethod, req, new(pb.UpdateSettingsRequest))
				require.Equal(t, codes.InvalidArgument, status.Code(err))
				detail := status.Convert(err).Details()[0].(*badrequestv1.BadRequest)
				got := make(map[string]string)
				for _, violation := range detail.GetFieldViolations() {
					got[violation.GetFieldPath()] = violation.GetCode()
				}
				require.Equal(t, map[string]string{tc.prefix: "required", tc.prefix + ".time_zone": "required"}, got)
			})
		}
	})

	t.Run("registered extractor miss publishes no selection", func(t *testing.T) {
		for _, caller := range []string{"", "nil"} {
			req := &pb.UpdateSettingsRequest{Caller: caller, Settings: &pb.Settings{}}
			require.NoError(t, conn.Invoke(t.Context(), unlocatedMethod, req, new(pb.UpdateSettingsRequest)))
		}
	})

	t.Run("required and Buf paths both use proto names", func(t *testing.T) {
		req := &pb.UpdateSettingsRequest{Settings: &pb.Settings{General: &pb.General{RetentionDays: 3651}},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"general.timeZone", "general.retentionDays"}}}
		err := conn.Invoke(t.Context(), scopedMethod, req, new(pb.UpdateSettingsRequest))
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		detail := status.Convert(err).Details()[0].(*badrequestv1.BadRequest)
		require.Len(t, detail.GetFieldViolations(), 2)
		require.Equal(t, "settings.general.time_zone", detail.GetFieldViolations()[0].GetFieldPath())
		require.Equal(t, "settings.general.retention_days", detail.GetFieldViolations()[1].GetFieldPath())
	})

	t.Run("legacy extractor miss keeps indexed handler error", func(t *testing.T) {
		for index, paths := range [][]string{{"bogus"}, {"enabled", "hits7d"}} {
			req := &pb.UpdateSettingsRequest{OutsideRequired: new("valid"), Settings: &pb.Settings{},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: paths}}
			err := conn.Invoke(t.Context(), legacyUnlocatedMethod, req, new(pb.UpdateSettingsRequest))
			require.True(t, proto.Equal(status.Convert(indexedTestMaskError(index)).Proto(), status.Convert(err).Proto()), "%v", err)
		}
	})
	t.Run("extractor returning detached resource is internal", func(t *testing.T) {
		req := &pb.UpdateSettingsRequest{Settings: &pb.Settings{}}
		err := conn.Invoke(t.Context(), brokenMethod, req, new(pb.UpdateSettingsRequest))
		require.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("legacy required keeps relative path and message", func(t *testing.T) {
		req := &pb.UpdateSettingsRequest{OutsideRequired: new("valid"), Settings: &pb.Settings{General: &pb.General{}},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"general.time_zone"}}}
		err := conn.Invoke(t.Context(), "/"+service+"/UpdateSettings", req, new(pb.UpdateSettingsRequest))
		require.Equal(t, "field behavior violation", status.Convert(err).Message())
		checkMaskValidationViolation(t, err, "general.time_zone", "")
		detail := status.Convert(err).Details()[0].(*badrequestv1.BadRequest)
		require.Equal(t, "required field must be set to a non-empty value", detail.GetFieldViolations()[0].GetMessage())
	})
	t.Run("legacy mask error precedes invalid identity", func(t *testing.T) {
		req := &pb.UpdateSettingsRequest{OutsideRequired: new("valid"), Settings: &pb.Settings{Id: "invalid"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"bogus"}}}
		err := conn.Invoke(t.Context(), "/"+service+"/UpdateSettings", req, new(pb.UpdateSettingsRequest))
		st := status.Convert(err)
		require.Equal(t, codes.InvalidArgument, st.Code())
		require.Len(t, st.Details(), 1)
		detail, ok := st.Details()[0].(*errdetails.BadRequest)
		require.True(t, ok, "%T", st.Details()[0])
		require.Len(t, detail.GetFieldViolations(), 1)
		require.Equal(t, "update_mask", detail.GetFieldViolations()[0].GetField())
		want := "invalid field mask path 'bogus': field 'bogus' does not exist in message maskvalidationtest.v1.Settings"
		require.Equal(t, want, st.Message())
		require.Equal(t, want, detail.GetFieldViolations()[0].GetDescription())
	})
	t.Run("outside required keeps Buf rule", func(t *testing.T) {
		for _, method := range []string{scopedMethod, "/" + service + "/UpdateSettings"} {
			req := &pb.UpdateSettingsRequest{OutsideRequired: new(""), Settings: &pb.Settings{}, UpdateMask: &fieldmaskpb.FieldMask{}}
			err := conn.Invoke(t.Context(), method, req, new(pb.UpdateSettingsRequest))
			checkMaskValidationViolation(t, err, "outside_required", "string.min_len")
		}
	})
	t.Run("outside format is checked", func(t *testing.T) {
		req := &pb.UpdateSettingsRequest{OutsideRequired: new("valid"), OutsideUuid: "invalid", Settings: &pb.Settings{},
			UpdateMask: &fieldmaskpb.FieldMask{}}
		err := conn.Invoke(t.Context(), scopedMethod, req, new(pb.UpdateSettingsRequest))
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		require.Contains(t, status.Convert(err).Details()[0].(*badrequestv1.BadRequest).GetFieldViolations()[0].GetFieldPath(), "outside_uuid")
	})

	t.Run("valid body reaches registered handler with mask error", func(t *testing.T) {
		req := &pb.UpdateSettingsRequest{
			OutsideRequired: new("valid"),
			Settings:        &pb.Settings{Synonyms: "valid", General: &pb.General{TimeZone: "UTC"}},
			UpdateMask:      &fieldmaskpb.FieldMask{Paths: []string{"etag"}},
		}
		before := extractions.Load()
		err := conn.Invoke(t.Context(), registeredMethod, req, new(pb.UpdateSettingsRequest))
		require.Equal(t, before+1, extractions.Load(), "both mask interceptors use one extraction")
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		require.Contains(t, status.Convert(err).Message(), "updateMask.paths[0]")
		require.Empty(t, status.Convert(err).Details())
	})

	t.Run("stream unchanged", func(t *testing.T) {
		s, err := conn.NewStream(t.Context(), &grpc.StreamDesc{ClientStreams: true}, "/"+service+"/UpdateStream")
		require.NoError(t, err)
		require.NoError(t, s.SendMsg(&pb.UpdateSettingsRequest{
			OutsideRequired: new("valid"),
			Settings:        &pb.Settings{General: &pb.General{RetentionDays: 9}},
			UpdateMask:      &fieldmaskpb.FieldMask{Paths: []string{"general.retentionDays"}},
		}))
		require.NoError(t, s.CloseSend())
		checkMaskValidationViolation(t, s.RecvMsg(new(pb.UpdateSettingsRequest)), "settings.general.time_zone", "string.min_len")
	})
}

func checkMaskValidationViolation(t *testing.T, err error, path, rule string) {
	t.Helper()
	st := status.Convert(err)
	require.Equal(t, codes.InvalidArgument, st.Code())
	for _, detail := range st.Details() {
		if bad, ok := detail.(*badrequestv1.BadRequest); ok {
			require.Len(t, bad.GetFieldViolations(), 1)
			require.Equal(t, path, bad.GetFieldViolations()[0].GetFieldPath())
			require.Equal(t, rule, bad.GetFieldViolations()[0].GetCode())
			return
		}
	}
	t.Fatalf("no BadRequest in %v", st.Details())
}

type settingsBehavior struct{}

func (settingsBehavior) Resolve(fd protoreflect.FieldDescriptor) []annotations.FieldBehavior {
	switch fd.FullName() {
	case "maskvalidationtest.v1.General.time_zone", "maskvalidationtest.v1.Settings.server_note":
		return nil
	case "maskvalidationtest.v1.Settings.format_uuid":
		return []annotations.FieldBehavior{annotations.FieldBehavior_OUTPUT_ONLY}
	}
	return (aipbehavior.AnnotationResolver{}).Resolve(fd)
}

func indexedTestMaskError(index int) error {
	path := fmt.Sprintf("updateMask.paths[%d]", index)
	st, err := status.New(codes.InvalidArgument, "invalid field mask").WithDetails(&errdetails.ErrorInfo{Reason: "INVALID_FIELD_MASK"},
		&badrequestv1.BadRequest{FieldViolations: []*badrequestv1.FieldViolation{
			{FieldPath: &path, Code: new("FIELD_BEHAVIOR_VIOLATION"), Message: new("rejected mask path " + strconv.Itoa(index))},
		}})
	if err != nil {
		panic(err)
	}
	return st.Err()
}

func TestRegisteredMaskRefusalCannotReachHandler(t *testing.T) {
	t.Parallel()
	const service = "maskvalidationtest.v1.SettingsService"
	const method = "/" + service + "/UpdateSettings"
	spec, err := update.NewSpec(new(pb.Settings))
	require.NoError(t, err)
	for _, ignore := range []bool{false, true} {
		name := "validator absent"
		if ignore {
			name = "validator ignores method"
		}
		t.Run(name, func(t *testing.T) {
			builder := New(&grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
				FieldMask: &grpcconfig.FieldMaskInterceptor{BaseInterceptor: enabled},
			}}).UseFieldMaskOptions(fieldmask.WithUpdateSpec(method, spec)).
				WithFieldBehaviorInterceptor().WithFieldMaskInterceptor()
			if ignore {
				builder.interceptors = append(builder.interceptors, &bufValidatorRegistration{
					protovalidator.ServerInterceptor(protovalidator.ValidatorFunc(bufhelpers.BuildValidator(nil)),
						protovalidator.WithIgnoreMethods(method)),
				})
			}
			conn := serveService(t, builder, settingsService(service, "UpdateSettings", func(context.Context, any) (any, error) {
				return nil, status.Error(codes.Aborted, "handler reached with unresolved mask")
			}))
			req := &pb.UpdateSettingsRequest{Settings: &pb.Settings{}, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"bogus"}}}
			err := conn.Invoke(t.Context(), method, req, new(pb.UpdateSettingsRequest))
			require.Equal(t, codes.InvalidArgument, status.Code(err))
			require.IsType(t, &errdetails.BadRequest{}, status.Convert(err).Details()[0])
		})
	}
}

func TestRegisteredSecondNormalizerEnforcesStrictMask(t *testing.T) {
	t.Parallel()
	const service = "maskvalidationtest.v1.SettingsService"
	const method = "/" + service + "/UpdateSettings"
	spec, err := update.NewSpec(new(pb.Settings))
	require.NoError(t, err)
	registration := fieldmask.WithUpdateSpec(method, spec)
	builder := New(&grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		FieldMask: &grpcconfig.FieldMaskInterceptor{BaseInterceptor: enabled},
	}}).UseFieldBehaviorOptions(fieldbehavior.WithUpdateNormalizer(fieldmask.UpdateNormalizer(registration))).
		UseFieldMaskOptions(registration, fieldmask.WithStrictMode()).WithInterceptors()
	conn := serveService(t, builder, settingsService(service, "UpdateSettings", func(_ context.Context, req any) (any, error) {
		return req, nil
	}))
	for _, mask := range []*fieldmaskpb.FieldMask{nil, {}} {
		req := &pb.UpdateSettingsRequest{Settings: &pb.Settings{}, OutsideRequired: new("valid"), UpdateMask: mask}
		err := conn.Invoke(t.Context(), method, req, new(pb.UpdateSettingsRequest))
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		require.IsType(t, &errdetails.BadRequest{}, status.Convert(err).Details()[0])
	}
}

func settingsService(service, method string, handler grpc.UnaryHandler) *grpc.ServiceDesc {
	return &grpc.ServiceDesc{ServiceName: service, HandlerType: (*any)(nil), Methods: []grpc.MethodDesc{{
		MethodName: method,
		Handler: func(srv any, ctx context.Context, dec func(any) error, ic grpc.UnaryServerInterceptor) (any, error) {
			req := new(pb.UpdateSettingsRequest)
			if err := dec(req); err != nil {
				return nil, err
			}
			return ic(ctx, req, &grpc.UnaryServerInfo{Server: srv, FullMethod: "/" + service + "/" + method}, handler)
		},
	}}}
}

type maskAfterNoop struct{ interceptors.ServerInterceptor }

func (i maskAfterNoop) Dependencies() []string {
	return []string{(&interceptors.NoOpInterceptor{}).Name()}
}

func TestMaskErrorDeferralUsesInstalledOrder(t *testing.T) {
	t.Parallel()
	const service = "maskvalidationtest.v1.SettingsService"
	const method = "/" + service + "/UpdateSettings"
	spec, err := update.NewSpec(new(pb.Settings))
	require.NoError(t, err)
	builder := New(&grpcconfig.Config{}).UseFieldMaskOptions(fieldmask.WithUpdateSpec(method, spec)).
		WithFieldBehaviorInterceptor().WithBufValidatorInterceptor()
	builder.interceptors[0] = maskAfterNoop{builder.interceptors[0]}
	builder.interceptors = append(builder.interceptors, &interceptors.NoOpInterceptor{})
	conn := serveService(t, builder, settingsService(service, "UpdateSettings", func(context.Context, any) (any, error) {
		return nil, status.Error(codes.Aborted, "handler reached with unresolved mask")
	}))
	req := &pb.UpdateSettingsRequest{Settings: &pb.Settings{Synonyms: "valid"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"bogus"}}}
	err = conn.Invoke(t.Context(), method, req, new(pb.UpdateSettingsRequest))
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.IsType(t, &errdetails.BadRequest{}, status.Convert(err).Details()[0])
}
