// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisutils_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/internal/redisutils"
)

func BenchmarkKeyBuilder_Build(b *testing.B) {
	kb := redisutils.NewKeyBuilder("myapp:cache")
	for b.Loop() {
		kb.Build("user:123")
	}
}

func BenchmarkKeyBuilder_BuildMany(b *testing.B) {
	kb := redisutils.NewKeyBuilder("myapp:cache")
	keys := []string{"key1", "key2", "key3", "key4", "key5"}
	for b.Loop() {
		kb.BuildMany(keys)
	}
}

func BenchmarkKeyBuilder_Pattern(b *testing.B) {
	kb := redisutils.NewKeyBuilder("myapp:cache")
	for b.Loop() {
		kb.Pattern()
	}
}
