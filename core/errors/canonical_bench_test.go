// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errors_test

import (
	"fmt"
	"testing"

	"github.com/altessa-s/go-atlas/core/errors"
)

func BenchmarkMapFieldPath(b *testing.B) {
	err := fmt.Errorf("create: %w", errors.NewFieldError("base_url", "required"))
	mapPath := func(f string) string { return "provider." + f }
	b.ReportAllocs()
	for b.Loop() {
		_ = errors.MapFieldPath(err, mapPath)
	}
}
