// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import (
	"testing"

	"google.golang.org/grpc/codes"
)

func FuzzCodeToString(f *testing.F) {
	f.Add(uint32(0))
	f.Add(uint32(16))
	f.Add(uint32(999))
	f.Fuzz(func(t *testing.T, code uint32) {
		result := CodeToString(codes.Code(code))
		if result == "" {
			t.Fatal("should never return empty string")
		}
	})
}
