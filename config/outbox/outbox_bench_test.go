// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxconfig_test

import (
	"testing"

	outboxconfig "github.com/altessa-s/go-atlas/config/outbox"
)

func BenchmarkValidate(b *testing.B) {
	cfg := outboxconfig.Default()
	for b.Loop() {
		if err := cfg.Validate(); err != nil {
			b.Fatal(err)
		}
	}
}
