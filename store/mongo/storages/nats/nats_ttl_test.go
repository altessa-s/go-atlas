// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/store/mongo"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	mongohelpers "github.com/altessa-s/go-atlas/store/mongo/internal/testhelpers"
	cursnats "github.com/altessa-s/go-atlas/store/mongo/storages/nats"
)

// TestNew_RejectsBucketWithoutTTL pins that a bucket whose keys never expire
// is refused: nothing deletes cursors, so it would grow without bound.
func TestNew_RejectsBucketWithoutTTL(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	kv := natstest.CreateNATSKV(t, js, "no-ttl", 0)

	s, err := cursnats.New(t.Context(), kv)
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	require.Nil(t, s)
}

// TestStorage_EntriesExpire pins that the bucket's key TTL removes cursors.
func TestStorage_EntriesExpire(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	kv := natstest.CreateNATSKV(t, js, "short-lived", time.Second)

	s, err := cursnats.New(t.Context(), kv)
	require.NoError(t, err)
	require.NoError(t, s.Store(t.Context(), "page-1", mongohelpers.SampleCursorMetadata()))

	_, err = s.Load(t.Context(), "page-1")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		_, err := s.Load(t.Context(), "page-1")
		return errors.Is(err, mongo.ErrCursorNotFound)
	}, 15*time.Second, 100*time.Millisecond, "the cursor outlived the bucket's key TTL")
}
