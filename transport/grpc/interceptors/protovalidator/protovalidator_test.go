// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package protovalidator

import (
	"context"
	"errors"
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestServerInterceptor_NilValidator(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("ServerInterceptor did not panic with nil validator")
		} else if r != "protovalidator: validator cannot be nil" {
			t.Errorf("unexpected panic message: %v", r)
		}
	}()

	ServerInterceptor(nil)
}

func TestServerInterceptor_Name(t *testing.T) {
	validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		return nil
	})

	i := ServerInterceptor(validator)
	if i.Name() != "protovalidator" {
		t.Fatalf("Name() = %q, want %q", i.Name(), "protovalidator")
	}
}

func TestServerInterceptor_Dependencies(t *testing.T) {
	validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		return nil
	})

	si := ServerInterceptor(validator)
	dsi, ok := si.(*interceptors.DrivenServerInterceptor)
	if !ok {
		t.Fatal("expected DrivenServerInterceptor")
	}

	ic, ok := dsi.Interceptor().(*interceptor)
	if !ok {
		t.Fatal("expected *interceptor")
	}

	deps := ic.Dependencies()
	if len(deps) != 1 {
		t.Fatalf("Dependencies len = %d, want 1", len(deps))
	}
	if deps[0] != "metadata" {
		t.Fatalf("deps[0] = %q, want %q", deps[0], "metadata")
	}
}

func TestServerInterceptor_ReturnsInterceptors(t *testing.T) {
	validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		return nil
	})

	i := ServerInterceptor(validator)
	if i.ServerUnaryInterceptor() == nil {
		t.Fatal("ServerUnaryInterceptor should not be nil")
	}
	if i.ServerStreamInterceptor() == nil {
		t.Fatal("ServerStreamInterceptor should not be nil")
	}
}

func TestValidate_PanicRecovery(t *testing.T) {
	panicValidator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		panic("test panic")
	})

	si := ServerInterceptor(panicValidator)
	unaryInt := si.ServerUnaryInterceptor()

	handler := func(ctx context.Context, req any) (any, error) {
		return &emptypb.Empty{}, nil
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	ctx := t.Context()
	_, err := unaryInt(ctx, &emptypb.Empty{}, info, handler)
	if err == nil {
		t.Fatal("expected error from panic recovery")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}

	if st.Code() != codes.Internal {
		t.Errorf("expected Internal code, got %v", st.Code())
	}

	if st.Message() != "Internal Error" {
		t.Errorf("unexpected error message: %v", st.Message())
	}
}

func TestValidate_NonProtoMessage(t *testing.T) {
	validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		t.Fatal("validator should not be called for non-proto message")
		return nil
	})

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptor("protovalidator", nil),
		validator:       validator,
		opts:            defaultOptions(),
	}

	ctx := t.Context()
	d, ctx := ic.DrivenInterceptor(ctx)
	ri := d.(*requestInterceptor)

	req := "not a proto message"
	err := ri.validate(ctx, req)
	if err != nil {
		t.Errorf("expected nil error for non-proto message, got %v", err)
	}
}

func TestValidate_ErrorWrapping(t *testing.T) {
	tests := []struct {
		name         string
		validatorErr error
		wantCode     codes.Code
		wantContains string
	}{
		{
			name:         "plain error",
			validatorErr: errors.New("validation error"),
			wantCode:     codes.InvalidArgument,
			wantContains: "validation failed: validation error",
		},
		{
			name:         "gRPC status error",
			validatorErr: status.Error(codes.PermissionDenied, "access denied"),
			wantCode:     codes.PermissionDenied,
			wantContains: "access denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
				return tt.validatorErr
			})

			ic := &interceptor{
				BaseInterceptor: interceptors.NewBaseInterceptor("protovalidator", nil),
				validator:       validator,
				opts:            defaultOptions(),
			}

			ctx := t.Context()
			d, ctx := ic.DrivenInterceptor(ctx)
			ri := d.(*requestInterceptor)

			req := &emptypb.Empty{}
			err := ri.validate(ctx, req)
			if err == nil {
				t.Fatal("expected error")
			}

			st, ok := status.FromError(err)
			if !ok {
				t.Fatal("expected gRPC status error")
			}

			if st.Code() != tt.wantCode {
				t.Errorf("expected code %v, got %v", tt.wantCode, st.Code())
			}

			if !contains(st.Message(), tt.wantContains) {
				t.Errorf("expected message to contain %q, got %q", tt.wantContains, st.Message())
			}
		})
	}
}

func TestValidate_IgnoreMethods(t *testing.T) {
	callCount := 0
	validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		callCount++
		return nil
	})

	// Test with method in ignore list
	callCount = 0
	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			"protovalidator",
			[]string{"/test.service/ignoredmethod"},
			nil,
			nil,
		),
		validator: validator,
		opts:      defaultOptions(),
	}

	ctx := t.Context()
	ctx = metadata.NewContext(ctx, &metadata.CallMetadata{
		FullyMethodName: "/test.Service/IgnoredMethod",
	})

	d, ctx := ic.DrivenInterceptor(ctx)
	ri := d.(*requestInterceptor)

	err := ri.validate(ctx, &emptypb.Empty{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 0 {
		t.Errorf("expected validator not to be called, got %d calls", callCount)
	}

	// Test with method NOT in ignore list
	callCount = 0
	ctx2 := t.Context()
	ctx2 = metadata.NewContext(ctx2, &metadata.CallMetadata{
		FullyMethodName: "/test.Service/AllowedMethod",
	})

	d2, ctx2 := ic.DrivenInterceptor(ctx2)
	ri2 := d2.(*requestInterceptor)

	err = ri2.validate(ctx2, &emptypb.Empty{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected validator to be called once, got %d", callCount)
	}
}

func TestServerUnaryInterceptor(t *testing.T) {
	validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		if _, ok := msg.(*emptypb.Empty); ok {
			return errors.New("empty not allowed")
		}
		return nil
	})

	interceptor := ServerInterceptor(validator)
	unaryInt := interceptor.ServerUnaryInterceptor()

	handler := func(ctx context.Context, req any) (any, error) {
		return &emptypb.Empty{}, nil
	}

	info := &grpc.UnaryServerInfo{
		Server:     nil,
		FullMethod: "/test.Service/Method",
	}

	// Test validation failure
	ctx := t.Context()
	_, err := unaryInt(ctx, &emptypb.Empty{}, info, handler)
	if err == nil {
		t.Fatal("expected validation error")
	}

	// Test validation success
	_, err = unaryInt(ctx, &wrapperspb.StringValue{Value: "test"}, info, handler)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPreCall_ClientSide(t *testing.T) {
	callCount := 0
	validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		callCount++
		return nil
	})

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptor("protovalidator", nil),
		validator:       validator,
		opts:            defaultOptions(),
	}

	ctx := t.Context()
	ctx = metadata.NewContext(ctx, &metadata.CallMetadata{
		FullyMethodName: "/test.Service/Method",
		IsClient:        true,
	})

	d, ctx := ic.DrivenInterceptor(ctx)
	ri := d.(*requestInterceptor)

	resp, err := ri.PreCall(ctx, &emptypb.Empty{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Error("expected nil response")
	}
	if callCount != 0 {
		t.Error("validator should not be called on client side")
	}
}

func TestPostMsgReceive_ValidatesStreamingMessages(t *testing.T) {
	validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		if _, ok := msg.(*emptypb.Empty); ok {
			return errors.New("empty not allowed in stream")
		}
		return nil
	})

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptor("protovalidator", nil),
		validator:       validator,
		opts:            defaultOptions(),
	}

	ctx := t.Context()
	ctx = metadata.NewContext(ctx, &metadata.CallMetadata{
		FullyMethodName: "/test.Service/StreamMethod",
	})

	d, ctx := ic.DrivenInterceptor(ctx)
	ri := d.(*requestInterceptor)

	// Test with valid message
	err := ri.PostMsgReceive(ctx, &wrapperspb.StringValue{Value: "test"}, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Test with invalid message
	err = ri.PostMsgReceive(ctx, &emptypb.Empty{}, nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestPostMsgReceive_PassesThroughExistingError(t *testing.T) {
	validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		t.Fatal("validator should not be called when there's an existing error")
		return nil
	})

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptor("protovalidator", nil),
		validator:       validator,
		opts:            defaultOptions(),
	}

	ctx := t.Context()
	d, ctx := ic.DrivenInterceptor(ctx)
	ri := d.(*requestInterceptor)

	existingErr := errors.New("existing error")
	err := ri.PostMsgReceive(ctx, &emptypb.Empty{}, existingErr)
	if err != existingErr {
		t.Errorf("expected existing error to be passed through, got %v", err)
	}
}

func TestPostCall_PassesThroughError(t *testing.T) {
	validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		return nil
	})

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptor("protovalidator", nil),
		validator:       validator,
		opts:            defaultOptions(),
	}

	ctx := t.Context()
	d, ctx := ic.DrivenInterceptor(ctx)
	ri := d.(*requestInterceptor)

	existingErr := errors.New("handler error")
	err := ri.PostCall(ctx, &emptypb.Empty{}, existingErr)
	if err != existingErr {
		t.Errorf("expected error to be passed through, got %v", err)
	}

	// Test nil error
	err = ri.PostCall(ctx, &emptypb.Empty{}, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestPostMsgSent_PassesThroughError(t *testing.T) {
	validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		return nil
	})

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptor("protovalidator", nil),
		validator:       validator,
		opts:            defaultOptions(),
	}

	ctx := t.Context()
	d, ctx := ic.DrivenInterceptor(ctx)
	ri := d.(*requestInterceptor)

	existingErr := errors.New("send error")
	err := ri.PostMsgSent(ctx, &emptypb.Empty{}, existingErr)
	if err != existingErr {
		t.Errorf("expected error to be passed through, got %v", err)
	}
}

func TestValidatorFunc(t *testing.T) {
	called := false
	vf := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		called = true
		return nil
	})

	err := vf.Validate(t.Context(), &emptypb.Empty{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("ValidatorFunc was not called")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr) != -1))
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
