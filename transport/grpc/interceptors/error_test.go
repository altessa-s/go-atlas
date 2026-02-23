// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNewError(t *testing.T) {
	tests := []struct {
		name    string
		code    codes.Code
		msg     string
		err     error
		wantMsg string
	}{
		{"with_error", codes.InvalidArgument, "bad request", errors.New("validation failed"), "validation failed"},
		{"nil_error_uses_status_msg", codes.Unauthenticated, "Missing Credentials", nil, "missing credentials"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := status.New(tt.code, tt.msg)
			e := NewError(st, tt.err)
			if e.Error() != tt.wantMsg {
				t.Fatalf("Error() = %q, want %q", e.Error(), tt.wantMsg)
			}
			if e.GRPCStatus().Code() != tt.code {
				t.Fatalf("code = %v, want %v", e.GRPCStatus().Code(), tt.code)
			}
		})
	}
}

func TestError_Unwrap(t *testing.T) {
	inner := errors.New("inner")
	e := NewError(status.New(codes.Internal, "fail"), inner)
	if !errors.Is(e, inner) {
		t.Fatal("Unwrap should return inner error")
	}
}

func TestError_Is(t *testing.T) {
	sentinel := errors.New("sentinel")
	e := NewError(status.New(codes.Internal, "fail"), sentinel)
	if !e.Is(sentinel) {
		t.Fatal("Is should match sentinel")
	}
	if e.Is(errors.New("other")) {
		t.Fatal("Is should not match different error")
	}
}

func TestError_GRPCStatus(t *testing.T) {
	st := status.New(codes.PermissionDenied, "denied")
	e := NewError(st, nil)
	if e.GRPCStatus() != st {
		t.Fatal("GRPCStatus should return original status")
	}
}
