// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func BenchmarkStaticToken_GetRequestMetadata(b *testing.B) {
	st := NewStaticToken("my-token")
	ctx := b.Context()
	for b.Loop() {
		st.GetRequestMetadata(ctx)
	}
}

func BenchmarkParseError(b *testing.B) {
	st := status.New(codes.NotFound, "not found")
	err := st.Err()
	for b.Loop() {
		ParseError(err)
	}
}

func BenchmarkError_IsNotFound(b *testing.B) {
	e := &Error{grpcCode: codes.NotFound}
	for b.Loop() {
		e.IsNotFound()
	}
}

func BenchmarkFieldError_Error(b *testing.B) {
	fe := &FieldError{Field: "email", Message: "invalid"}
	var s string
	for b.Loop() {
		s = fe.Error()
	}
	_ = s
}
