// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package freecache_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/altessa-s/go-atlas/data/cache/providers/freecache"
)

func FuzzProvider_SaveGet(f *testing.F) {
	f.Add([]byte("key"), []byte("value"))
	f.Add([]byte(""), []byte(""))
	f.Add([]byte("k"), []byte("v"))
	f.Add([]byte("long-key-name-with-many-characters"), []byte("long-value-with-many-characters"))

	f.Fuzz(func(t *testing.T, key []byte, value []byte) {
		p := freecache.New()
		ctx := t.Context()

		keyStr := string(key)
		err := p.Save(ctx, keyStr, value, 10*time.Second)
		if err != nil {
			return
		}

		got, err := p.Get(ctx, keyStr)
		if err != nil {
			return
		}

		assert.Equal(t, string(value), string(got))
	})
}
