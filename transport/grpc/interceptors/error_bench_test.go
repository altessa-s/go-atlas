// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func BenchmarkNewError(b *testing.B) {
	st := status.New(codes.Internal, "error")
	for b.Loop() {
		NewError(st, nil)
	}
}

func BenchmarkError_Error(b *testing.B) {
	e := NewError(status.New(codes.Internal, "error"), nil)
	for b.Loop() {
		_ = e.Error()
	}
}

func BenchmarkError_GRPCStatus(b *testing.B) {
	e := NewError(status.New(codes.Internal, "error"), nil)
	for b.Loop() {
		_ = e.GRPCStatus()
	}
}
