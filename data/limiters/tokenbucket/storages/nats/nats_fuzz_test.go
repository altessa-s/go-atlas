// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	limitnats "github.com/altessa-s/go-atlas/data/limiters/tokenbucket/storages/nats"
)

func FuzzProvider_Allow(f *testing.F) {
	f.Add("key1", int64(10), int64(60))
	f.Add("special-key", int64(1), int64(1))

	ns := testhelpers.StartNATSServer(f)
	_, js := testhelpers.ConnectJetStream(f, ns)

	provider, err := limitnats.New(js, limitnats.WithBucket("fuzz-limiter"))
	if err != nil {
		f.Fatalf("failed to create provider: %v", err)
	}

	ctx := f.Context()

	f.Fuzz(func(t *testing.T, key string, limit, periodSec int64) {
		if key == "" || limit <= 0 || periodSec <= 0 {
			return
		}
		period := time.Duration(periodSec) * time.Second
		if period <= 0 {
			return
		}
		// Should not panic
		_, _ = provider.Allow(ctx, key, limit, period)
	})
}
