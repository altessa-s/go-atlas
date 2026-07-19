// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/altessa-s/go-atlas/data/cache/providers/redis"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func FuzzProvider_SaveGet(f *testing.F) {
	// Add seed corpus
	f.Add("key1", []byte("value1"))
	f.Add("key2", []byte("value2"))
	f.Add("", []byte(""))
	f.Add("special:key:with:colons", []byte("special value"))
	f.Add("unicode-key-🔑", []byte("unicode-value-📦"))

	f.Fuzz(func(t *testing.T, key string, value []byte) {
		client, _ := testhelpers.RedisClient(t)

		provider := redis.New(client)
		ctx := t.Context()

		// Save should not panic
		err := provider.Save(ctx, key, value, 10*time.Second)
		if err != nil {
			// It's okay for some operations to fail (e.g., invalid keys),
			// but they should not panic
			return
		}

		// Get should not panic
		got, err := provider.Get(ctx, key)
		if err != nil {
			// It's okay for Get to fail, but it should not panic
			return
		}

		// If both Save and Get succeeded, values should match
		assert.Equal(t, string(value), string(got))
	})
}
