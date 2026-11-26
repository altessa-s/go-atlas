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

func BenchmarkErrorText(b *testing.B) {
	const raw = "dial postgres://admin:s3cr/et@db.internal:5432/app?password=secret&sslmode=disable"
	for b.Loop() {
		diagnostics.ErrorText(raw, diagnostics.DefaultMaxErrorLength)
	}
}
