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

	"github.com/altessa-s/go-atlas/coord/dlock/providers"
	"github.com/altessa-s/go-atlas/coord/dlock/providers/providertest"
	"github.com/altessa-s/go-atlas/internal/nats/natskvlease"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	locknats "github.com/altessa-s/go-atlas/coord/dlock/providers/nats"
)

// TestProviderContract runs the provider contract suite with one embedded
// server and bucket per contract. Removing the key simulates server expiry.
func TestProviderContract(t *testing.T) {
	t.Parallel()
	providertest.Run(t, func(tb testing.TB) providertest.Backend {
		ns := natstest.StartNATSServer(tb)
		bucket := strings.ReplaceAll(tb.Name(), "/", "-")
		_, js := natstest.ConnectJetStream(tb, ns)
		return providertest.Backend{
			NewProvider: func(tb testing.TB) providers.Provider {
				locker, err := locknats.New(tb.Context(), natstest.ConnectNATS(tb, ns), locknats.WithBucket(bucket),
					locknats.WithTTL(time.Second))
				require.NoError(tb, err)
				tb.Cleanup(func() { _ = locker.Close(context.WithoutCancel(tb.Context())) })
				return locker
			},
			Expire: func(tb testing.TB, key string) {
				kv, err := js.KeyValue(tb.Context(), bucket)
				require.NoError(tb, err)
				require.NoError(tb, natskvlease.NewKVOps(kv, nil).Delete(tb.Context(), key))
			},
		}
	})
}
