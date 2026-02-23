// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"errors"
	"testing"

	"golang.org/x/oauth2"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStaticToken_GetRequestMetadata(t *testing.T) {
	st := NewStaticToken("my-token")
	md, err := st.GetRequestMetadata(t.Context())
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if got := md["authorization"]; got != "Bearer my-token" {
		t.Fatalf("authorization = %q", got)
	}
}

func TestStaticToken_RequireTransportSecurity(t *testing.T) {
	st := NewStaticToken("tok")
	if !st.RequireTransportSecurity() {
		t.Fatal("RequireTransportSecurity() = false")
	}
}

func TestInsecureTokenCredentials_GetRequestMetadata(t *testing.T) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "oauth-token"})
	creds := NewInsecureTokenCredentials(ts)

	md, err := creds.GetRequestMetadata(t.Context())
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if got := md["authorization"]; got != "Bearer oauth-token" {
		t.Fatalf("authorization = %q", got)
	}
}

func TestInsecureTokenCredentials_RequireTransportSecurity(t *testing.T) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{})
	creds := NewInsecureTokenCredentials(ts)
	if creds.RequireTransportSecurity() {
		t.Fatal("RequireTransportSecurity() = true")
	}
}

func TestFieldError_Error(t *testing.T) {
	fe := &FieldError{Field: "email", Message: "invalid format"}
	if got := fe.Error(); got != "[email]: invalid format" {
		t.Fatalf("Error() = %q", got)
	}
}

func TestNewFieldError(t *testing.T) {
	fe := &FieldError{Field: "name", Message: "required"}
	if fe.Field != "name" {
		t.Fatalf("Field = %q", fe.Field)
	}
	if fe.Message != "required" {
		t.Fatalf("Message = %q", fe.Message)
	}
}

func TestError_CodeChecks(t *testing.T) {
	tests := []struct {
		code   codes.Code
		method string
		want   bool
	}{
		{codes.NotFound, "IsNotFound", true},
		{codes.AlreadyExists, "IsAlreadyExists", true},
		{codes.PermissionDenied, "IsPermissionDenied", true},
		{codes.Unauthenticated, "IsUnauthenticated", true},
		{codes.Internal, "IsInternal", true},
		{codes.Unavailable, "IsUnavailable", true},
		{codes.FailedPrecondition, "IsFailedPrecondition", true},
		{codes.DeadlineExceeded, "IsDeadlineExceeded", true},
		{codes.Canceled, "IsCanceled", true},
		{codes.ResourceExhausted, "IsResourceExhausted", true},
		{codes.InvalidArgument, "IsInvalidArgument", true},
		{codes.Aborted, "IsAborted", true},
		{codes.OutOfRange, "IsOutOfRange", true},
		{codes.Unimplemented, "IsUnimplemented", true},
		{codes.DataLoss, "IsDataLoss", true},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			e := &Error{grpcCode: tt.code}
			if e.Code() != tt.code {
				t.Fatalf("Code() = %v", e.Code())
			}
		})
	}
}

func TestError_IsNotFound(t *testing.T) {
	e := &Error{grpcCode: codes.NotFound}
	if !e.IsNotFound() {
		t.Fatal("IsNotFound() = false")
	}
	e2 := &Error{grpcCode: codes.Internal}
	if e2.IsNotFound() {
		t.Fatal("Internal.IsNotFound() = true")
	}
}

func TestError_IsValidationError(t *testing.T) {
	e := &Error{grpcCode: codes.InvalidArgument, Fields: []FieldError{{Field: "f", Message: "m"}}}
	if !e.IsValidationError() {
		t.Fatal("IsValidationError() = false")
	}

	e2 := &Error{grpcCode: codes.InvalidArgument}
	if e2.IsValidationError() {
		t.Fatal("no fields should not be validation error")
	}
}

func TestError_ReasonIs(t *testing.T) {
	e := &Error{Reason: "USER_NOT_FOUND"}
	if !e.ReasonIs("USER_NOT_FOUND") {
		t.Fatal("ReasonIs() = false")
	}
	if e.ReasonIs("OTHER") {
		t.Fatal("ReasonIs(OTHER) = true")
	}
}

func TestError_ReasonIsOneof(t *testing.T) {
	e := &Error{Reason: "B"}
	if !e.ReasonIsOneof("A", "B", "C") {
		t.Fatal("ReasonIsOneof() = false")
	}
	if e.ReasonIsOneof("X", "Y") {
		t.Fatal("ReasonIsOneof(X,Y) = true")
	}
}

func TestError_HasFields(t *testing.T) {
	e := &Error{Fields: []FieldError{{Field: "f"}}}
	if !e.HasFields() {
		t.Fatal("HasFields() = false")
	}
	e2 := &Error{}
	if e2.HasFields() {
		t.Fatal("empty.HasFields() = true")
	}
}

func TestError_GetField(t *testing.T) {
	e := &Error{Fields: []FieldError{
		{Field: "email", Message: "invalid"},
		{Field: "name", Message: "required"},
	}}

	f := e.GetField("email")
	if f == nil || f.Message != "invalid" {
		t.Fatalf("GetField(email) = %v", f)
	}

	if e.GetField("missing") != nil {
		t.Fatal("GetField(missing) should return nil")
	}
}

func TestError_Error_String(t *testing.T) {
	t.Run("with_message", func(t *testing.T) {
		e := &Error{grpcCode: codes.NotFound, Message: "item not found"}
		got := e.Error()
		if got != "NotFound: item not found" {
			t.Fatalf("Error() = %q", got)
		}
	})

	t.Run("validation", func(t *testing.T) {
		e := &Error{grpcCode: codes.InvalidArgument, Fields: []FieldError{
			{Field: "email", Message: "invalid"},
		}}
		got := e.Error()
		if got != "validation error: [email]: invalid" {
			t.Fatalf("Error() = %q", got)
		}
	})
}

func TestError_GRPCStatus(t *testing.T) {
	e := &Error{grpcCode: codes.NotFound, Message: "not found"}
	st := e.GRPCStatus()
	if st.Code() != codes.NotFound {
		t.Fatalf("Code() = %v", st.Code())
	}
}

func TestParseError_Nil(t *testing.T) {
	if ParseError(nil) != nil {
		t.Fatal("ParseError(nil) should return nil")
	}
}

func TestParseError_NonGRPC(t *testing.T) {
	if ParseError(errors.New("plain error")) != nil {
		t.Fatal("ParseError(plain) should return nil")
	}
}

func TestParseError_GRPC(t *testing.T) {
	st := status.New(codes.NotFound, "not found")
	e := ParseError(st.Err())
	if e == nil {
		t.Fatal("ParseError returned nil")
	}
	if !e.IsNotFound() {
		t.Fatal("IsNotFound() = false")
	}
}

func TestIsClientError(t *testing.T) {
	e := &Error{grpcCode: codes.NotFound}
	if !IsClientError(e) {
		t.Fatal("IsClientError() = false")
	}
	if IsClientError(errors.New("plain")) {
		t.Fatal("IsClientError(plain) = true")
	}
}

func TestAsClientError(t *testing.T) {
	e := &Error{grpcCode: codes.NotFound}
	got := AsClientError(e)
	if got == nil {
		t.Fatal("AsClientError() = nil")
	}
	if AsClientError(errors.New("plain")) != nil {
		t.Fatal("AsClientError(plain) should return nil")
	}
}
