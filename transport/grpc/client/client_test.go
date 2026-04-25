// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"golang.org/x/oauth2"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStaticToken_GetRequestMetadata(t *testing.T) {
	st := NewStaticToken("my-token")
	md, err := st.GetRequestMetadata(t.Context())
	require.NoError(t, err)
	got := md["authorization"]
	require.Equal(t, "Bearer my-token", got)
}

func TestStaticToken_RequireTransportSecurity(t *testing.T) {
	st := NewStaticToken("tok")
	require.True(t, st.RequireTransportSecurity(), "RequireTransportSecurity() = false")
}

func TestInsecureTokenCredentials_GetRequestMetadata(t *testing.T) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "oauth-token"})
	creds := NewInsecureTokenCredentials(ts)

	md, err := creds.GetRequestMetadata(t.Context())
	require.NoError(t, err)
	got := md["authorization"]
	require.Equal(t, "Bearer oauth-token", got)
}

func TestInsecureTokenCredentials_RequireTransportSecurity(t *testing.T) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{})
	creds := NewInsecureTokenCredentials(ts)
	require.False(t, creds.RequireTransportSecurity(), "RequireTransportSecurity() = true")
}

func TestFieldError_Error(t *testing.T) {
	fe := &FieldError{Field: "email", Message: "invalid format"}
	got := fe.Error()
	require.Equal(t, "[email]: invalid format", got)
}

func TestNewFieldError(t *testing.T) {
	fe := &FieldError{Field: "name", Message: "required"}
	require.Equal(t, "name", fe.Field)
	require.Equal(t, "required", fe.Message)
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
			require.Equal(t, tt.code, e.Code())
		})
	}
}

func TestError_IsNotFound(t *testing.T) {
	e := &Error{grpcCode: codes.NotFound}
	require.True(t, e.IsNotFound(), "IsNotFound() = false")
	e2 := &Error{grpcCode: codes.Internal}
	require.False(t, e2.IsNotFound(), "Internal.IsNotFound() = true")
}

func TestError_IsValidationError(t *testing.T) {
	e := &Error{grpcCode: codes.InvalidArgument, Fields: []FieldError{{Field: "f", Message: "m"}}}
	require.True(t, e.IsValidationError(), "IsValidationError() = false")

	e2 := &Error{grpcCode: codes.InvalidArgument}
	require.False(t, e2.IsValidationError(), "no fields should not be validation error")
}

func TestError_ReasonIs(t *testing.T) {
	e := &Error{Reason: "USER_NOT_FOUND"}
	require.True(t, e.ReasonIs("USER_NOT_FOUND"), "ReasonIs() = false")
	require.False(t, e.ReasonIs("OTHER"), "ReasonIs(OTHER) = true")
}

func TestError_ReasonIsOneof(t *testing.T) {
	e := &Error{Reason: "B"}
	require.True(t, e.ReasonIsOneof("A", "B", "C"), "ReasonIsOneof() = false")
	require.False(t, e.ReasonIsOneof("X", "Y"), "ReasonIsOneof(X,Y) = true")
}

func TestError_HasFields(t *testing.T) {
	e := &Error{Fields: []FieldError{{Field: "f"}}}
	require.True(t, e.HasFields(), "HasFields() = false")
	e2 := &Error{}
	require.False(t, e2.HasFields(), "empty.HasFields() = true")
}

func TestError_GetField(t *testing.T) {
	e := &Error{Fields: []FieldError{
		{Field: "email", Message: "invalid"},
		{Field: "name", Message: "required"},
	}}

	f := e.GetField("email")
	require.NotNil(t, f, "GetField(email) = %v", f)
	require.Equal(t, "invalid", f.Message)

	require.Nil(t, e.GetField("missing"))
}

func TestError_Error_String(t *testing.T) {
	t.Run("with_message", func(t *testing.T) {
		e := &Error{grpcCode: codes.NotFound, Message: "item not found"}
		got := e.Error()
		require.Equal(t, "NotFound: item not found", got)
	})

	t.Run("validation", func(t *testing.T) {
		e := &Error{grpcCode: codes.InvalidArgument, Fields: []FieldError{
			{Field: "email", Message: "invalid"},
		}}
		got := e.Error()
		require.Equal(t, "validation error: [email]: invalid", got)
	})
}

func TestError_GRPCStatus(t *testing.T) {
	e := &Error{grpcCode: codes.NotFound, Message: "not found"}
	st := e.GRPCStatus()
	require.Equal(t, codes.NotFound, st.Code())
}

func TestParseError_Nil(t *testing.T) {
	require.Nil(t, ParseError(nil))
}

func TestParseError_NonGRPC(t *testing.T) {
	require.Nil(t, ParseError(errors.New("plain error")))
}

func TestParseError_GRPC(t *testing.T) {
	st := status.New(codes.NotFound, "not found")
	e := ParseError(st.Err())
	require.NotNil(t, e, "ParseError returned nil")
	require.True(t, e.IsNotFound(), "IsNotFound() = false")
}

func TestIsClientError(t *testing.T) {
	e := &Error{grpcCode: codes.NotFound}
	require.True(t, IsClientError(e), "IsClientError() = false")
	require.False(t, IsClientError(errors.New("plain")), "IsClientError(plain) = true")
}

func TestAsClientError(t *testing.T) {
	e := &Error{grpcCode: codes.NotFound}
	got := AsClientError(e)
	require.NotNil(t, got, "AsClientError() = nil")
	require.Nil(t, AsClientError(errors.New("plain")))
}
