// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/fieldmask/update"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/updatemask"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	pb "github.com/altessa-s/go-atlas/proto/gen/maskvalidationtest/v1"
)

func TestAtomicOneofMaskFactoryChain(t *testing.T) {
	t.Parallel()

	const (
		service = "maskvalidationtest.v1.IntegrationService"
		method  = "/" + service + "/UpdateIntegration"
	)
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

	selections := make(chan updatemask.Selection, 1)
	handle := func(ctx context.Context, raw any) (any, error) {
		req := raw.(*pb.UpdateIntegrationRequest)
		sel, ok := updatemask.SelectionFromContext(ctx, req)
		if !ok || sel.Spec != spec || sel.Resource != req.Integration {
			return nil, status.Error(codes.Internal, "missing registered selection")
		}
		if sel.Err != nil {
			if pathErr, ok := errors.AsType[*update.MaskPathError](sel.Err); ok {
				return nil, indexedTestMaskError(pathErr.Index)
			}
			return nil, sel.Err
		}
		select {
		case selections <- sel:
			return req, nil
		default:
			return nil, status.Error(codes.Internal, "previous selection was not read")
		}
	}
	builder := New(&grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		FieldMask: &grpcconfig.FieldMaskInterceptor{BaseInterceptor: enabled},
	}}).UseReasonCode(func(rule, _ string) string { return rule }).
		UseFieldMaskOptions(fieldmask.WithUpdateSpec(method, spec)).WithInterceptors()
	conn := serveService(t, builder, &grpc.ServiceDesc{
		ServiceName: service,
		HandlerType: (*any)(nil),
		Methods: []grpc.MethodDesc{{
			MethodName: "UpdateIntegration",
			Handler: func(srv any, ctx context.Context, dec func(any) error, ic grpc.UnaryServerInterceptor) (any, error) {
				req := new(pb.UpdateIntegrationRequest)
				if err := dec(req); err != nil {
					return nil, err
				}
				return ic(ctx, req, &grpc.UnaryServerInfo{Server: srv, FullMethod: method}, handle)
			},
		}},
	})

	mask := func(paths ...string) *fieldmaskpb.FieldMask { return &fieldmaskpb.FieldMask{Paths: paths} }
	const implied = update.SynthesizedPathIndex
	const smtpKind = pb.Integration_KIND_SMTP
	identity := &pb.Integration{Kind: smtpKind, Etag: "v1"}
	emptySMTP := &pb.Integration{Kind: smtpKind, Etag: "v1", Settings: &pb.Integration_Smtp{Smtp: &pb.SmtpSettings{}}}
	validSMTP := &pb.Integration{Kind: smtpKind, Etag: "v1", Name: "mail", Enabled: true,
		Settings: &pb.Integration_Smtp{Smtp: &pb.SmtpSettings{Host: "relay"}}}
	invalidPort := &pb.Integration{Kind: smtpKind, Etag: "v1",
		Settings: &pb.Integration_Smtp{Smtp: &pb.SmtpSettings{Host: "relay", Port: 65536}}}
	foreign := &pb.Integration{Kind: smtpKind, Etag: "v1",
		Settings: &pb.Integration_Telegram{Telegram: &pb.TelegramSettings{BotToken: "token"}}}

	for _, tt := range []struct {
		name        string
		body        *pb.Integration
		mask        *fieldmaskpb.FieldMask
		paths       []update.ResolvedPath
		field, rule string
	}{
		{name: "omitted selects populated scalars and whole branch", body: validSMTP, paths: []update.ResolvedPath{
			{Path: "name", Index: implied}, {Path: "enabled", Index: implied}, {Path: "smtp", Index: implied},
		}},
		{name: "omitted with no mutable values", body: identity},
		{name: "omitted validates whole branch", body: emptySMTP, field: "integration.smtp.host", rule: "required"},
		{name: "omitted foreign branch has synthesized origin", body: foreign,
			paths: []update.ResolvedPath{{Path: "telegram", Index: implied}}},
		{name: "explicit foreign branch has client index", body: validSMTP, mask: mask("name", "telegram"),
			paths: []update.ResolvedPath{{Path: "name", Index: 0}, {Path: "telegram", Index: 1}}},
		{name: "explicit matching branch absent", body: identity, mask: mask("smtp"),
			paths: []update.ResolvedPath{{Path: "smtp", Index: 0}}},
		{name: "explicit matching branch with foreign body", body: foreign, mask: mask("smtp"),
			paths: []update.ResolvedPath{{Path: "smtp", Index: 0}}},
		{name: "explicit branch requires host", body: emptySMTP, mask: mask("smtp"), field: "integration.smtp.host", rule: "required"},
		{name: "explicit branch validates populated fields", body: invalidPort, mask: mask("smtp"), field: "integration.smtp.port", rule: "int32.lte"},
		{name: "explicit name and branch", body: validSMTP, mask: mask("name", "smtp"),
			paths: []update.ResolvedPath{{Path: "name", Index: 0}, {Path: "smtp", Index: 1}}},
		{name: "explicit zeros and duplicate paths", body: identity, mask: mask("enabled", "name", "enabled"),
			paths: []update.ResolvedPath{{Path: "name", Index: 1}, {Path: "enabled", Index: 0}}},
		{name: "enabled needs no settings", body: identity, mask: mask("enabled"), paths: []update.ResolvedPath{{Path: "enabled", Index: 0}}},
		{name: "enabled ignores unselected invalid settings", body: emptySMTP, mask: mask("enabled"),
			paths: []update.ResolvedPath{{Path: "enabled", Index: 0}}},
		{name: "empty selects no mutable fields", body: emptySMTP, mask: mask()},
		{name: "wildcard validates present branch", body: emptySMTP, mask: mask("*"), field: "integration.smtp.host", rule: "required"},
		{name: "wildcard ignores absent sibling", body: validSMTP, mask: mask("*")},
		{name: "wildcard leaves missing branch for handler", body: identity, mask: mask("*")},
		{name: "missing integration", field: "integration", rule: "required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := &pb.UpdateIntegrationRequest{Integration: tt.body, UpdateMask: tt.mask}
			reply := new(pb.UpdateIntegrationRequest)
			err := conn.Invoke(t.Context(), method, req, reply)
			if tt.rule != "" {
				require.Empty(t, selections)
				checkMaskValidationViolation(t, err, tt.field, tt.rule)
				return
			}
			require.NoError(t, err)
			select {
			case sel := <-selections:
				require.Equal(t, tt.mask != nil, sel.OriginalMask() != nil)
				require.Equal(t, tt.mask.GetPaths(), sel.OriginalMask().GetPaths())
				if tt.mask == nil || len(tt.mask.Paths) != 1 || tt.mask.Paths[0] != "*" {
					require.Equal(t, tt.paths, sel.Mask.Paths())
				}
			default:
				t.Fatal("handler did not receive a selection")
			}
			require.True(t, proto.Equal(tt.body, reply.Integration), "handler received %v, want %v", reply.Integration, tt.body)
		})
	}

	for _, path := range []string{"smtp.host", "telegram.botToken", "kind", "etag", "id", "lastCheck", "last_check", "Name", "unknown", "*"} {
		t.Run("refuse/"+path, func(t *testing.T) {
			req := &pb.UpdateIntegrationRequest{Integration: emptySMTP, UpdateMask: mask("name", path)}
			err := conn.Invoke(t.Context(), method, req, new(pb.UpdateIntegrationRequest))
			checkMaskValidationViolation(t, err, "updateMask.paths[1]", "FIELD_BEHAVIOR_VIOLATION")
			for _, detail := range status.Convert(err).Details() {
				if info, ok := detail.(*errdetails.ErrorInfo); ok {
					require.Equal(t, "INVALID_FIELD_MASK", info.GetReason())
					return
				}
			}
			t.Fatal("missing ErrorInfo")
		})
	}

	for _, tt := range []struct {
		name string
		mask *fieldmaskpb.FieldMask
	}{
		{name: "omitted"},
		{name: "explicit", mask: mask("enabled")},
		{name: "empty", mask: mask()},
		{name: "wildcard", mask: mask("*")},
		{name: "unresolved", mask: mask("smtp.host")},
	} {
		t.Run("identity/"+tt.name, func(t *testing.T) {
			for _, bad := range []struct {
				body        *pb.Integration
				field, rule string
			}{
				{body: &pb.Integration{Etag: "v1"}, field: "kind", rule: "enum.not_in"},
				{body: &pb.Integration{Kind: pb.Integration_Kind(99), Etag: "v1"}, field: "kind", rule: "enum.defined_only"},
				{body: &pb.Integration{Kind: smtpKind, Etag: "too long"}, field: "etag", rule: "string.max_len"},
			} {
				t.Run(fmt.Sprintf("%s/%s", bad.field, bad.rule), func(t *testing.T) {
					req := &pb.UpdateIntegrationRequest{Integration: bad.body, UpdateMask: tt.mask}
					err := conn.Invoke(t.Context(), method, req, new(pb.UpdateIntegrationRequest))
					require.Empty(t, selections)
					checkMaskValidationViolation(t, err, "integration."+bad.field, bad.rule)
				})
			}
		})
	}
}
