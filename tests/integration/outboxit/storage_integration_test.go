// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/reliability/outbox"
	"github.com/altessa-s/go-atlas/reliability/outbox/storages/storagetest"
)

// The storage contract suite runs against MongoDB and every SQL backend.
func TestStorageContract(t *testing.T) {
	t.Parallel()

	t.Run("mongo", func(t *testing.T) {
		t.Parallel()
		newStore := func(tb testing.TB) outbox.Storage { return newFixture(tb).store }
		storagetest.RunWithOrderKeys(t, newStore, false)
	})
	for _, spec := range sqlSpecs() {
		t.Run(spec.name, func(t *testing.T) {
			t.Parallel()
			newStore := func(tb testing.TB) outbox.Storage { return newSQLFixture(tb, spec).store }
			storagetest.RunWithOrderKeys(t, newStore, true)
		})
	}
}

// The Watch contract runs on the backends that can serve it: MongoDB change
// streams, and PostgreSQL LISTEN/NOTIFY through the pgx listener.
func TestStorageContract_Watch(t *testing.T) {
	t.Parallel()

	t.Run("mongo", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.requireChangeStreams(t)
		storagetest.Watch(t, f.store)
	})
	t.Run("postgres", func(t *testing.T) {
		t.Parallel()
		storagetest.Watch(t, newNotifySQLFixture(t).store)
	})
}
