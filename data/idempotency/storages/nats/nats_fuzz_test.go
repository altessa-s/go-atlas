// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	idempnats "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
)

func FuzzStorage_AttemptLock(f *testing.F) {
	f.Add("key1", []byte("value1"))
	f.Add("", []byte(""))
	f.Add("special.key", []byte("data"))

	ns := testhelpers.StartNATSServer(f)
	_, js := testhelpers.ConnectJetStream(f, ns)

	storage, err := idempnats.New(js, idempnats.WithBucket("fuzz-idemp"))
	if err != nil {
		f.Fatalf("failed to create storage: %v", err)
	}

	ctx := f.Context()

	f.Fuzz(func(t *testing.T, key string, val []byte) {
		// NATS KV keys cannot contain '.', '*', '>' or be empty for non-empty-key path
		// Just verify no panics
		_, _, _, _ = storage.AttemptLock(ctx, key, val)
		_ = storage.Complete(ctx, key, val, nil)
		_ = storage.Delete(ctx, key)
	})
}
