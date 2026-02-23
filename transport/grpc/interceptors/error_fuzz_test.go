// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func FuzzNewError(f *testing.F) {
	f.Add("error message", int32(0))
	f.Add("", int32(2))
	f.Add("unicode: привет", int32(13))

	f.Fuzz(func(t *testing.T, msg string, code int32) {
		if code < 0 || code > 16 {
			return
		}
		st := status.New(codes.Code(code), msg)
		e := NewError(st, nil)
		if e.Error() == "" && msg != "" {
			t.Fatal("expected non-empty error")
		}
		if e.GRPCStatus() == nil {
			t.Fatal("GRPCStatus should not be nil")
		}
	})
}
