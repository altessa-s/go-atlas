// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/proto-gen-go/io/altessa/badrequest/v1"

	"github.com/altessa-s/go-atlas/transport/grpc/client"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/protoadapt"
)

func TestParseStatusError_BadRequestDetails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		detail protoadapt.MessageV1
		want   []client.FieldError
	}{
		{
			name: "altessa BadRequest from a go-atlas server",
			detail: &badrequestv1.BadRequest{FieldViolations: []*badrequestv1.FieldViolation{
				{FieldPath: new("user.email"), Message: new("value must be a valid email"), Code: new("EMAIL_INVALID")},
				{FieldPath: new("tenant_id"), Message: new("field is immutable")},
			}},
			want: []client.FieldError{
				{Field: "user.email", Message: "value must be a valid email", Code: "EMAIL_INVALID"},
				{Field: "tenant_id", Message: "field is immutable"},
			},
		},
		{
			name: "google.rpc.BadRequest from a third-party server",
			detail: &errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{
				{Field: "user.email", Description: "invalid"},
			}},
			want: []client.FieldError{{Field: "user.email", Message: "invalid"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			st, err := status.New(codes.InvalidArgument, "Validation Failed").WithDetails(tc.detail)
			require.NoError(t, err)

			got := client.ParseError(st.Err())
			require.NotNil(t, got)
			require.True(t, got.IsValidationError())
			require.Equal(t, tc.want, got.Fields)
		})
	}
}
