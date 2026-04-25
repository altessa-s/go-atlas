// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	uniqnats "github.com/altessa-s/go-atlas/data/uniq/providers/nats"
)

func FuzzProvider_AddExist(f *testing.F) {
	f.Add("key1", "value1")
	f.Add("special.key", "data")

	ns := testhelpers.StartNATSServer(f)
	nc := testhelpers.ConnectNATS(f, ns)

	p, err := uniqnats.New(nc, uniqnats.WithBucket("fuzz-uniq"))
	if err != nil {
		f.Fatalf("failed to create provider: %v", err)
	}
	ctx := f.Context()

	f.Fuzz(func(t *testing.T, key, value string) {
		if key == "" {
			return
		}
		// Should not panic
		_ = p.AddWithValue(ctx, key, []byte(value))
		_, _ = p.Exist(ctx, key)
		_, _ = p.GetValue(ctx, key)
	})
}
