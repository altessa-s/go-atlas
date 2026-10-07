// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/locks/dlock/providers"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/providertest"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	locknats "github.com/altessa-s/go-atlas/data/locks/dlock/providers/nats"
)

// TestProviderContract runs the provider contract suite with one embedded
// server and bucket per contract. NATS keeps the lease in the bucket's key
// TTL, which a test cannot age out from under the holder, so the expiry
// contracts are skipped.
func TestProviderContract(t *testing.T) {
	t.Parallel()
	providertest.Run(t, func(tb testing.TB) providertest.Backend {
		ns := testhelpers.StartNATSServer(tb)
		bucket := strings.ReplaceAll(tb.Name(), "/", "-")
		return providertest.Backend{NewProvider: func(tb testing.TB) providers.Provider {
			locker, err := locknats.New(tb.Context(), testhelpers.ConnectNATS(tb, ns), locknats.WithBucket(bucket),
				locknats.WithTTL(time.Second))
			require.NoError(tb, err)
			tb.Cleanup(func() { _ = locker.Close(context.Background()) })
			return locker
		}}
	})
}
