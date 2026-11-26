// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/coord/limiters/budget"
)

func BenchmarkBuild_Memory(b *testing.B) {
	cfg := validConfig()
	cfg.InsecureEvictLRU = true
	for b.Loop() {
		_, err := factory.New(cfg).Build(b.Context())
		require.NoError(b, err)
	}
}
