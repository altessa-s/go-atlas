// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisutils_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/internal/redisutils"
)

func FuzzKeyBuilder_Build(f *testing.F) {
	f.Add("prefix", "key")
	f.Add("", "key")
	f.Add("prefix", "")
	f.Add("", "")
	f.Add("a:b:c:", "d:e:f")

	f.Fuzz(func(t *testing.T, prefix, key string) {
		kb := redisutils.NewKeyBuilder(prefix)
		_ = kb.Build(key)
		_ = kb.Pattern()
		_ = kb.HasPrefix()
		_ = kb.Prefix()
	})
}
