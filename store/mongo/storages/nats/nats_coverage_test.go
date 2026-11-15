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

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/store/mongo"

	mongohelpers "github.com/altessa-s/go-atlas/store/mongo/internal/testhelpers"
	cursnats "github.com/altessa-s/go-atlas/store/mongo/storages/nats"
)

// The embedded JSON storage's own Delete goes through the backend, which
// purges the key like the Storage override does.
func TestStorage_EmbeddedJSONDelete(t *testing.T) {
	t.Parallel()
	s := setupStorage(t)
	ctx := t.Context()
	require.NoError(t, s.Store(ctx, "k", mongohelpers.SampleCursorMetadata()))

	require.NoError(t, s.JSONStorage.Delete(ctx, "k"))
	_, err := s.Load(ctx, "k")
	require.ErrorIs(t, err, mongo.ErrCursorNotFound)
	require.NoError(t, s.JSONStorage.Delete(ctx, "k"), "deleting a missing key is idempotent")
}

func TestStorage_OverlongKeyIsRejectedEverywhere(t *testing.T) {
	t.Parallel()
	s := setupStorage(t)
	ctx := t.Context()
	key := strings.Repeat("k", 4096)

	require.Error(t, s.Delete(ctx, key))
	require.Error(t, s.JSONStorage.Delete(ctx, key))
	_, err := s.Load(ctx, key)
	require.Error(t, err)
}

func TestStorage_ErrorsAfterServerShutdown(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	kv := natstest.CreateNATSKV(t, js, "shutdown", time.Hour)
	ctx := t.Context()
	s, err := cursnats.New(ctx, kv)
	require.NoError(t, err)
	require.NoError(t, s.Ping(ctx))
	require.NoError(t, s.Store(ctx, "k", mongohelpers.SampleCursorMetadata()))

	ns.Shutdown()
	ns.WaitForShutdown()
	ctx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()

	require.Error(t, s.Ping(ctx))
	require.Error(t, s.Delete(ctx, "k"))
	require.Error(t, s.JSONStorage.Delete(ctx, "k"))
	_, err = s.Load(ctx, "k")
	require.Error(t, err)
	require.NotErrorIs(t, err, mongo.ErrCursorNotFound, "a transport failure is not a missing cursor")
}
