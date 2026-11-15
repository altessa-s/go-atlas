// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/fieldmask/extract"
	"github.com/altessa-s/go-aip/fieldmask/update"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"

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

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func updateRequestDescriptor(t *testing.T, maskName string) protoreflect.MessageDescriptor {
	t.Helper()
	fd, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name: new("deferred_" + maskName + ".proto"), Package: new("deferred"), Syntax: new("proto3"),
		Dependency: []string{(&pb.Resource{}).ProtoReflect().Descriptor().ParentFile().Path(), fieldmaskpb.File_google_protobuf_field_mask_proto.Path()},
		MessageType: []*descriptorpb.DescriptorProto{{Name: new("Request"), Field: []*descriptorpb.FieldDescriptorProto{
			{Name: new("resource"), Number: new(int32(1)), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: new(".fieldbehaviortest.v1.Resource")},
			{Name: new(maskName), Number: new(int32(2)), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: new(".google.protobuf.FieldMask")},
		}}},
	}, protoregistry.GlobalFiles)
	require.NoError(t, err)
	return fd.Messages().Get(0)
}

// TestDeferredUpdateFactoryChain exercises wire decoding, factory options,
// both interceptor orders, handler-owned comparison, and persisted output.
func TestDeferredUpdateFactoryChain(t *testing.T) {
	for _, order := range []string{"fieldmask", "fieldbehavior", "behavior-mask", "mask-behavior"} {
		for _, maskName := range []string{"update_mask", "updateMask"} {
			t.Run(order+"/"+maskName, func(t *testing.T) {
				md := updateRequestDescriptor(t, maskName)
				resourceFD := md.Fields().ByName("resource")
				request := func(res *pb.Resource, paths []string) *dynamicpb.Message {
					req := dynamicpb.NewMessage(md)
					req.Set(resourceFD, protoreflect.ValueOfMessage(res.ProtoReflect()))
					if paths != nil {
						require.NoError(t, extract.SetUpdateMask(req, &fieldmaskpb.FieldMask{Paths: paths}))
					}
					return req
				}
				stored := &pb.Resource{Id: "id-1", Name: "stored", TenantId: "tenant-1", Description: "before", CreateTime: "server"}
				initial := proto.CloneOf(stored)
				var storeMu sync.Mutex
				spec, err := update.NewSpec(stored)
				require.NoError(t, err)
				var entered, saves, validated atomic.Int32
				var savedSecret atomic.Pointer[string]
				var handlerMaskEmpty atomic.Bool
				patch := update.Patch[proto.Message, proto.Message, *int64]{
					Field: "resource", Spec: spec,
					Precondition: update.Int64Precondition[proto.Message]{Version: func(proto.Message) int64 { return 1 }},
					Read: func(context.Context) (proto.Message, error) {
						storeMu.Lock()
						defer storeMu.Unlock()
						return proto.CloneOf(stored), nil
					},
					ToProto: func(m proto.Message) proto.Message { return m },
					Save: func(_ context.Context, m proto.Message, plan update.Plan, _ *int64) (proto.Message, error) {
						savedSecret.Store(plan.Secret("password", nil))
						storeMu.Lock()
						stored = proto.Clone(m).(*pb.Resource)
						storeMu.Unlock()
						saves.Add(1)
						return m, nil
					},
				}
				run := func(ctx context.Context, req any) (any, error) {
					entered.Add(1)
					mask, res, ok := extract.UpdateMask(req.(proto.Message))
					if !ok {
						return nil, status.Error(codes.Internal, "missing normalized mask")
					}
					if _, ok := update.PlanFromContext(ctx, res); !ok {
						return nil, status.Error(codes.Internal, "missing deferred plan")
					}
					handlerMaskEmpty.Store(len(mask.GetPaths()) == 0)
					return patch.Run(ctx, res, "", mask)
				}
				cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
					ErrStatus: &grpcconfig.ErrStatusInterceptor{Enabled: true},
					FieldMask: &grpcconfig.FieldMaskInterceptor{BaseInterceptor: enabled},
				}}
				b := New(cfg).UseErrStatusOptions(errstatus.WithCanonicalRules()).WithErrStatusInterceptor()
				switch order {
				case "fieldmask":
					b.WithFieldMaskInterceptor()
				case "fieldbehavior":
					b.WithFieldBehaviorInterceptor()
				case "behavior-mask":
					b.WithFieldBehaviorInterceptor().WithFieldMaskInterceptor()
				case "mask-behavior":
					b.WithFieldMaskInterceptor().WithFieldBehaviorInterceptor()
				}
				b.interceptors = append(b.interceptors, interceptors.ServerDrivenInterceptor(&preparedValidator{calls: &validated}))
				// A final driven wrapper with no context hook must preserve upstream plans.
				b.interceptors = append(b.interceptors, interceptors.ServerDrivenInterceptor(&planPassthrough{}))
				streamDesc := grpc.StreamDesc{StreamName: "UpdateStream", ClientStreams: true, Handler: func(_ any, stream grpc.ServerStream) error {
					var previous *update.DeferredPlan
					var previousContext context.Context
					var previousResource proto.Message
					var last any
					for {
						req := dynamicpb.NewMessage(md)
						if err := stream.RecvMsg(req); err == io.EOF {
							break
						} else if err != nil {
							return err
						}
						ctx := stream.Context()
						_, res, _ := extract.UpdateMask(req)
						pending, ok := update.PlanFromContext(ctx, res)
						if !ok || pending == previous {
							return status.Error(codes.Internal, "stream plan reused")
						}
						if previous != nil {
							if _, ok := update.PlanFromContext(ctx, previousResource); ok {
								return status.Error(codes.Internal, "old plan retained")
							}
							if old, ok := update.PlanFromContext(previousContext, previousResource); !ok || old != previous {
								return status.Error(codes.Internal, "context snapshot changed")
							}
						}
						previous, previousContext, previousResource = pending, ctx, res
						var err error
						last, err = run(ctx, req)
						if err != nil {
							return err
						}
					}
					return stream.SendMsg(last)
				}}
				conn := serveService(t, b, &grpc.ServiceDesc{ServiceName: "deferred.Service", HandlerType: (*any)(nil), Methods: []grpc.MethodDesc{{MethodName: "UpdateResource", Handler: func(srv any, ctx context.Context, dec func(any) error, ic grpc.UnaryServerInterceptor) (any, error) {
					req := dynamicpb.NewMessage(md)
					if err := dec(req); err != nil {
						return nil, err
					}
					return ic(ctx, req, &grpc.UnaryServerInfo{Server: srv, FullMethod: "/deferred.Service/UpdateResource"}, run)
				}}}, Streams: []grpc.StreamDesc{streamDesc}})
				for _, tc := range []struct {
					name        string
					paths       []string
					tenant      string
					code        codes.Code
					writes      int32
					description string
				}{
					{"narrow unchanged", []string{"description"}, "tenant-1", codes.OK, 1, "after"},
					{"changed explicit", []string{"tenant_id"}, "changed", codes.InvalidArgument, 0, ""},
					{"changed wildcard", []string{"*"}, "changed", codes.InvalidArgument, 0, ""},
					{"changed omitted", nil, "changed", codes.InvalidArgument, 0, ""},
					{"identifier unchanged", []string{"id"}, "tenant-1", codes.OK, 0, "before"},
					{"immutable clear", []string{"tenant_id"}, "", codes.InvalidArgument, 0, ""},
					{"output only", []string{"create_time"}, "tenant-1", codes.OK, 0, "before"},
					{"output and mutable", []string{"create_time", "description"}, "tenant-1", codes.OK, 1, "after"},
					{"empty", []string{}, "changed", codes.OK, 0, "before"},
					{"omitted implied", nil, "tenant-1", codes.OK, 1, "after"},
				} {
					t.Run(tc.name, func(t *testing.T) {
						storeMu.Lock()
						stored = proto.CloneOf(initial)
						storeMu.Unlock()
						input := proto.CloneOf(initial)
						input.Description = "after"
						input.TenantId = tc.tenant
						input.CreateTime = "client"
						beforeEntered, beforeSaves := entered.Load(), saves.Load()
						out := new(pb.Resource)
						err := conn.Invoke(t.Context(), "/deferred.Service/UpdateResource", request(input, tc.paths), out)
						require.Equal(t, tc.code, status.Code(err), "%v", err)
						require.Equal(t, beforeEntered+1, entered.Load(), "comparison must run in handler")
						require.Equal(t, beforeSaves+tc.writes, saves.Load())
						if tc.code != codes.OK {
							require.Contains(t, fmt.Sprint(status.Convert(err).Details()), "tenant_id")
							return
						}
						expected := proto.CloneOf(initial)
						expected.Description = tc.description
						require.True(t, proto.Equal(expected, out), "got %v, want %v", out, expected)
					})
				}
				// Every case crosses the wire and the downstream validator in both RPC modes.
				for _, tc := range []struct {
					name        string
					paths       []string
					output      string
					description string
					tenant      string
					password    string
					code        codes.Code
				}{
					{name: "empty invalid output", paths: []string{}, output: "invalid"},
					{name: "empty invalid mutable", paths: []string{}, description: "invalid"},
					{name: "output only invalid mutable", paths: []string{"create_time"}, output: "invalid", description: "invalid"},
					{name: "unchanged immutable invalid mutable", paths: []string{"tenant_id"}, description: "invalid"},
					{name: "selected invalid mutable", paths: []string{"description"}, description: "invalid", code: codes.InvalidArgument},
					{name: "changed immutable empty remainder", paths: []string{"tenant_id"}, tenant: "changed", description: "invalid", code: codes.InvalidArgument},
					{name: "selected secret", paths: []string{"password"}, password: "one-shot"},
					{name: "empty secret", paths: []string{}, password: "one-shot"},
				} {
					for _, streaming := range []bool{false, true} {
						t.Run(fmt.Sprintf("validating/%s/stream=%t", tc.name, streaming), func(t *testing.T) {
							storeMu.Lock()
							stored = proto.CloneOf(initial)
							storeMu.Unlock()
							savedSecret.Store(nil)
							input := proto.CloneOf(initial)
							input.CreateTime, input.Description, input.Password = tc.output, tc.description, tc.password
							if tc.tenant != "" {
								input.TenantId = tc.tenant
							}
							beforeEntered, beforeSaves, beforeValidated := entered.Load(), saves.Load(), validated.Load()
							out := new(pb.Resource)
							var err error
							if streaming {
								stream, streamErr := conn.NewStream(t.Context(), &streamDesc, "/deferred.Service/UpdateStream")
								require.NoError(t, streamErr)
								require.NoError(t, stream.SendMsg(request(input, tc.paths)))
								require.NoError(t, stream.CloseSend())
								err = stream.RecvMsg(out)
							} else {
								err = conn.Invoke(t.Context(), "/deferred.Service/UpdateResource", request(input, tc.paths), out)
							}
							require.Equal(t, tc.code, status.Code(err), "%v", err)
							require.Equal(t, beforeValidated+1, validated.Load(), "validator must see every prepared message")
							if tc.name == "selected invalid mutable" {
								require.Equal(t, beforeEntered, entered.Load(), "validator must reject before handler")
								require.Contains(t, status.Convert(err).Message(), "description")
							} else {
								require.Equal(t, beforeEntered+1, entered.Load())
								if tc.tenant == "changed" {
									require.True(t, handlerMaskEmpty.Load(), "protected comparison must survive an empty effective mask")
									require.Contains(t, fmt.Sprint(status.Convert(err).Details()), "tenant_id")
								}
							}
							if tc.name == "selected secret" {
								require.Equal(t, beforeSaves+1, saves.Load())
								require.NotNil(t, savedSecret.Load())
								require.Equal(t, tc.password, *savedSecret.Load(), "Plan.Secret must retain the captured input")
							} else {
								require.Equal(t, beforeSaves, saves.Load(), "no resource or secret write")
								require.Nil(t, savedSecret.Load())
							}
							if tc.code == codes.OK {
								require.True(t, proto.Equal(initial, out), "got %v, want stored %v", out, initial)
							}
							storeMu.Lock()
							persisted := proto.CloneOf(stored)
							storeMu.Unlock()
							require.True(t, proto.Equal(initial, persisted), "stored resource changed: %v", persisted)
						})
					}
				}
				storeMu.Lock()
				stored = proto.CloneOf(initial)
				storeMu.Unlock()
				stream, err := conn.NewStream(t.Context(), &streamDesc, "/deferred.Service/UpdateStream")
				require.NoError(t, err)
				beforeSaves := saves.Load()
				for _, paths := range [][]string{{"description"}, {"tenant_id"}} {
					res := proto.CloneOf(initial)
					res.Description = "after"
					if paths[0] == "tenant_id" {
						res.TenantId = ""
					}
					require.NoError(t, stream.SendMsg(request(res, paths)))
				}
				require.NoError(t, stream.CloseSend())
				err = stream.RecvMsg(new(pb.Resource))
				require.Equal(t, codes.InvalidArgument, status.Code(err), "%v", err)
				require.Contains(t, fmt.Sprint(status.Convert(err).Details()), "tenant_id")
				require.Equal(t, beforeSaves+1, saves.Load())
				expected := proto.CloneOf(initial)
				expected.Description = "after"
				storeMu.Lock()
				saved := proto.CloneOf(stored)
				storeMu.Unlock()
				require.True(t, proto.Equal(expected, saved), "stored %v, want %v", saved, expected)
			})
		}
	}
}

type planPassthrough struct{}

func (*planPassthrough) Name() string { return "plan-passthrough" }
func (*planPassthrough) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
	return driver.NoopDriver(), ctx
}

// preparedValidator rejects sentinel values only when they survive preparation.
// It runs after the mask interceptors, before the handler, for every received message.
type preparedValidator struct {
	calls *atomic.Int32
}

func (*preparedValidator) Name() string { return "prepared-validator" }
func (v *preparedValidator) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
	return &preparedValidationDriver{Driver: driver.NoopDriver(), calls: v.calls}, ctx
}

type preparedValidationDriver struct {
	driver.Driver
	calls *atomic.Int32
}

func (v *preparedValidationDriver) PreCall(_ context.Context, req any) (any, error) {
	return nil, v.validate(req)
}

func (v *preparedValidationDriver) PostMsgReceive(_ context.Context, req any, err error) error {
	if err != nil {
		return err
	}
	return v.validate(req)
}

func (*preparedValidationDriver) PostMsgSent(_ context.Context, _ any, err error) error {
	return err
}

func (v *preparedValidationDriver) validate(req any) error {
	// Stream setup invokes PreCall before any message has been received.
	if req == nil {
		return nil
	}
	v.calls.Add(1)
	_, resource, ok := extract.UpdateMask(req.(proto.Message))
	if !ok {
		return status.Error(codes.Internal, "validator received no resource")
	}
	m := resource.ProtoReflect()
	for _, name := range []protoreflect.Name{"create_time", "description"} {
		if m.Get(m.Descriptor().Fields().ByName(name)).String() == "invalid" {
			return status.Errorf(codes.InvalidArgument, "invalid %s", name)
		}
	}
	return nil
}
