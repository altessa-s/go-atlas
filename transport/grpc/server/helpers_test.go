// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import (
	"testing"

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
			if got := CodeToString(tt.code); got != tt.want {
				t.Fatalf("CodeToString(%d) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestAllCodes(t *testing.T) {
	if len(AllCodes) != 17 {
		t.Fatalf("len(AllCodes) = %d, want 17", len(AllCodes))
	}
	if AllCodes[0] != codes.OK {
		t.Fatal("first code should be OK")
	}
	if AllCodes[len(AllCodes)-1] != codes.Unauthenticated {
		t.Fatal("last code should be Unauthenticated")
	}
}

func TestAllCodes_AllHaveStrings(t *testing.T) {
	for _, c := range AllCodes {
		s := CodeToString(c)
		if s == "" {
			t.Fatalf("CodeToString(%d) returned empty", c)
		}
	}
}

func BenchmarkCodeToString(b *testing.B) {
	for b.Loop() {
		CodeToString(codes.OK)
	}
}
