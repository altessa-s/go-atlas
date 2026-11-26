// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/nats/natsbase"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/reliability/saga"
)

// hookKV runs a function after an instance key (not an index key) is deleted.
type hookKV struct {
	jetstream.KeyValue
	afterDelete func()
}

func (k *hookKV) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	err := k.KeyValue.Delete(ctx, key, opts...)
	if k.afterDelete != nil && !strings.HasPrefix(key, indexPrefix) {
		k.afterDelete()
	}
	return err
}

// A Delete racing a Create of the same ID must not remove the marker the new
// instance wrote: the instance would then be live and invisible to recovery.
func TestDeleteKeepsMarkerOfConcurrentlyRecreatedInstance(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	real, err := New(t.Context(), js, WithBucket("saga_race"))
	require.NoError(t, err)
	live := &saga.Instance{ID: "x", Definition: "d", Status: saga.StatusCompensating}
	require.NoError(t, real.Create(t.Context(), live))

	recreated := false
	racing := &Store{Base: natsbase.NewBase(&hookKV{KeyValue: real.KV(), afterDelete: func() {
		// Between Delete's instance removal and its marker cleanup.
		recreated = true
		require.NoError(t, real.Create(t.Context(), &saga.Instance{ID: "x", Definition: "d", Status: saga.StatusCompensating}))
	}}), opts: real.opts}
	require.NoError(t, racing.Delete(t.Context(), "x"))
	require.True(t, recreated)

	rec, err := real.FetchRecoverable(t.Context(), "d", time.Now(), 10)
	require.NoError(t, err)
	require.Len(t, rec, 1, "the recreated instance must stay indexed")
}
