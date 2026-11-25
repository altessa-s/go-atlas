// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package diagnostics_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/internal/diagnostics"
)

func BenchmarkErrorClass(b *testing.B) {
	for b.Loop() {
		diagnostics.ErrorClass(context.DeadlineExceeded)
	}
}
