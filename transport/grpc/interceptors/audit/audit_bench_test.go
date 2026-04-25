// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func BenchmarkClassifyGRPCStatus_Nil(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		classifyGRPCStatus(nil)
	}
}

func BenchmarkClassifyGRPCStatus_PermissionDenied(b *testing.B) {
	b.ReportAllocs()
	err := status.Error(codes.PermissionDenied, "access denied")
	b.ResetTimer()
	for b.Loop() {
		classifyGRPCStatus(err)
	}
}

func BenchmarkClassifyGRPCStatus_Internal(b *testing.B) {
	b.ReportAllocs()
	err := status.Error(codes.Internal, "internal error")
	b.ResetTimer()
	for b.Loop() {
		classifyGRPCStatus(err)
	}
}
