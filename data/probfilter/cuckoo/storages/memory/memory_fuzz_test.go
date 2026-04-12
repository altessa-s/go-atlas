// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	memory "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
)

func FuzzStorage_AddMightExist(f *testing.F) {
	f.Add("test")
	f.Add("")
	f.Add("special-chars-!@#$%")

	f.Fuzz(func(t *testing.T, value string) {
		storage := memory.New(memory.WithCapacity(1000))
		ctx := t.Context()

		err := storage.Add(ctx, value)
		assert.NoError(t, err)

		_, err = storage.MightExist(ctx, value)
		assert.NoError(t, err)
	})
}
