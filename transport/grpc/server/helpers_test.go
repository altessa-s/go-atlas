// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
)

func TestCodeToString(t *testing.T) {
	tests := []struct {
		code codes.Code
		want string
	}{
		{codes.OK, "Ok"},
		{codes.Canceled, "Canceled"},
		{codes.Unknown, "Unknown"},
		{codes.InvalidArgument, "Invalid Argument"},
		{codes.DeadlineExceeded, "Deadline Exceeded"},
		{codes.NotFound, "Not Found"},
		{codes.AlreadyExists, "Already Exists"},
		{codes.PermissionDenied, "Permission Denied"},
		{codes.ResourceExhausted, "Resource Exhausted"},
		{codes.FailedPrecondition, "Failed Precondition"},
		{codes.Aborted, "Aborted"},
		{codes.OutOfRange, "Out Of Range"},
		{codes.Unimplemented, "Unimplemented"},
		{codes.Internal, "Internal"},
		{codes.Unavailable, "Unavailable"},
		{codes.DataLoss, "Data Loss"},
		{codes.Unauthenticated, "Unauthenticated"},
		{codes.Code(999), "Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := CodeToString(tt.code)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestAllCodes(t *testing.T) {
	require.Len(t, AllCodes, 17)
	require.Equal(t, codes.OK, AllCodes[0])
	require.Equal(t, codes.Unauthenticated, AllCodes[len(AllCodes)-1])
}

func TestAllCodes_AllHaveStrings(t *testing.T) {
	for _, c := range AllCodes {
		s := CodeToString(c)
		require.NotEqual(t, "", s)
	}
}

func BenchmarkCodeToString(b *testing.B) {
	for b.Loop() {
		CodeToString(codes.OK)
	}
}
