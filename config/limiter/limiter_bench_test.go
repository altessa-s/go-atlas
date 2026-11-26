// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiterconfig_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
)

func BenchmarkBudget_Validate(b *testing.B) {
	cfg := limiterconfig.DefaultBudget()
	cfg.Limit = 1000
	cfg.Storage = memoryStorage()
	for b.Loop() {
		require.NoError(b, cfg.Validate())
	}
}
