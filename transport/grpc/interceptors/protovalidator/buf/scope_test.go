// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bufhelpers

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/fieldmask/update"

	"github.com/altessa-s/go-atlas/internal/transport/updatevalidation"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/maskvalidationtest/v1"
)

func TestBuildScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		paths               []string
		general             *pb.General
		timeZone, retention bool
		wholeGeneral        bool
		routes              bool
	}{
		{name: "leaf", paths: []string{"general.retentionDays"}, general: &pb.General{}, retention: true},
		{name: "parent", paths: []string{"general"}, general: &pb.General{}, timeZone: true, retention: true, wholeGeneral: true},
		{name: "absent parent", paths: []string{"general"}},
		{name: "wildcard", paths: []string{"*"}, general: &pb.General{}, timeZone: true, retention: true, wholeGeneral: true, routes: true},
		{name: "omitted", general: &pb.General{RetentionDays: 5}, retention: true, routes: true},
		{name: "empty", paths: []string{}, general: &pb.General{}},
		{name: "list", paths: []string{"notifications.routes"}, general: &pb.General{}, routes: true},
	}
	spec, err := update.NewSpec(new(pb.Settings))
	require.NoError(t, err)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := &pb.Settings{
				General: tc.general,
				Notifications: &pb.Notifications{Routes: []*pb.Route{
					{Kind: pb.Route_KIND_EMAIL, Nested: &pb.General{TimeZone: "UTC"}},
					{Nested: &pb.General{}},
				}},
			}
			var raw *fieldmaskpb.FieldMask
			if tc.paths != nil {
				raw = &fieldmaskpb.FieldMask{Paths: tc.paths}
			}
			resolved, err := spec.ResolveMask(res, raw)
			require.NoError(t, err)
			sel := updatevalidation.Selection{Resource: res, Spec: spec}
			scope, err := buildSelectedScope(res.ProtoReflect(), resolved.Paths(), nil, &sel)
			require.NoError(t, err)
			_, generalScoped := scope.instances[res.General]
			require.Equal(t, tc.general != nil && (tc.timeZone || tc.retention), generalScoped)
			if generalScoped {
				msg := res.General.ProtoReflect()
				require.Equal(t, tc.timeZone, scope.allows(msg, testScopeField(msg, "time_zone")))
				require.Equal(t, tc.retention, scope.allows(msg, testScopeField(msg, "retention_days")))
				require.Equal(t, tc.wholeGeneral, scope.allows(msg, msg.Descriptor()))
			}
			for _, route := range res.Notifications.Routes {
				_, routeScoped := scope.instances[route]
				_, nestedScoped := scope.instances[route.Nested]
				require.Equal(t, tc.routes, routeScoped)
				require.Equal(t, tc.routes, nestedScoped)
				require.Equal(t, tc.routes, scope.allows(route.ProtoReflect(), testScopeField(route.ProtoReflect(), "kind")))
				require.Equal(t, tc.routes, scope.allows(route.Nested.ProtoReflect(), route.Nested.ProtoReflect().Descriptor()))
			}
			for _, name := range []protoreflect.Name{"id", "etag"} {
				msg := res.ProtoReflect()
				require.True(t, scope.allows(msg, testScopeField(msg, name)))
			}
		})
	}
}

func testScopeField(msg protoreflect.Message, name protoreflect.Name) protoreflect.FieldDescriptor {
	return msg.Descriptor().Fields().ByName(name)
}

func TestBuildScopeForAtomicOneof(t *testing.T) {
	t.Parallel()

	spec, err := update.NewSpec(new(pb.Integration),
		update.WithAllowedPaths("name", "enabled", "smtp", "telegram"),
		update.WithIdentityFields("kind", "etag"),
		update.WithAtomicMessage(func(md protoreflect.MessageDescriptor) bool {
			switch md.FullName() {
			case "maskvalidationtest.v1.SmtpSettings", "maskvalidationtest.v1.TelegramSettings":
				return true
			default:
				return update.DefaultAtomicMessage(md)
			}
		}))
	require.NoError(t, err)

	res := &pb.Integration{
		Kind:     pb.Integration_KIND_SMTP,
		Etag:     "v1",
		Settings: &pb.Integration_Smtp{Smtp: &pb.SmtpSettings{}},
	}
	for _, tt := range []struct {
		name  string
		mask  *fieldmaskpb.FieldMask
		whole bool
	}{
		{name: "omitted", whole: true},
		{name: "explicit branch", mask: &fieldmaskpb.FieldMask{Paths: []string{"smtp"}}, whole: true},
		{name: "wildcard", mask: &fieldmaskpb.FieldMask{Paths: []string{"*"}}, whole: true},
		{name: "unselected branch", mask: &fieldmaskpb.FieldMask{Paths: []string{"enabled"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mask, err := spec.ResolveMask(res, tt.mask)
			require.NoError(t, err)
			sel := &updatevalidation.Selection{Resource: res, Spec: spec, Mask: mask}
			scope, err := buildSelectedScope(res.ProtoReflect(), mask.Paths(), nil, sel)
			require.NoError(t, err)

			branch := res.GetSmtp().ProtoReflect()
			require.Equal(t, tt.whole, scope.allows(branch, branch.Descriptor()))
			require.Equal(t, tt.whole, scope.allows(branch, testScopeField(branch, "host")))
			require.Equal(t, tt.whole, scope.allows(branch, testScopeField(branch, "port")))
			wantInstances := 1
			if tt.whole {
				wantInstances++
			}
			require.Len(t, scope.instances, wantInstances)
		})
	}
}

func TestValidateUpdateRejectsMismatchedSpec(t *testing.T) {
	t.Parallel()
	msg := &pb.Settings{}
	spec, err := update.NewSpec(msg)
	require.NoError(t, err)
	other, err := update.NewSpec(msg)
	require.NoError(t, err)
	mask, err := spec.ResolveMask(msg, &fieldmaskpb.FieldMask{})
	require.NoError(t, err)

	validator, err := newValidator()
	require.NoError(t, err)
	sel := &updatevalidation.Selection{Request: msg, Resource: msg, Spec: other, Mask: mask}
	require.Equal(t, codes.Internal, status.Code(validateUpdate(validator, msg, nil, sel)))
}
