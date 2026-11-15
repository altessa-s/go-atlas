// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import (
	"testing"

	"google.golang.org/grpc/codes"
)

func BenchmarkCodeToString(b *testing.B) {
	for b.Loop() {
		CodeToString(codes.OK)
	}
}
