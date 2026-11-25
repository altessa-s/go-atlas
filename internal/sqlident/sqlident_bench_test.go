// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqlident_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/internal/sqlident"
)

func BenchmarkValidate(b *testing.B) {
	for b.Loop() {
		_ = sqlident.Validate("app.created_at")
	}
}
