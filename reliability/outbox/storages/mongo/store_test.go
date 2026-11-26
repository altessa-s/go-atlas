// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"

	"github.com/altessa-s/go-atlas/reliability/outbox"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	mongoevent "go.mongodb.org/mongo-driver/v2/event"
	mongooptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestUpdateEventsReleasesClaimWithoutWritingAttemptHistory(t *testing.T) {
	t.Parallel()

	var update bson.Raw
	opts := mongooptions.Client().SetMonitor(&mongoevent.CommandMonitor{
		Started: func(_ context.Context, evt *mongoevent.CommandStartedEvent) {
			if evt.CommandName == "update" {
				update = append(bson.Raw(nil), evt.Command...)
			}
		},
	})
	opts.Deployment = drivertest.NewMockDeployment(bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}})
	client, err := mongo.Connect(opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Disconnect(context.WithoutCancel(t.Context()))) })
	store := &Store{collection: client.Database("test").Collection("outbox")}

	require.NoError(t, store.UpdateEvents(t.Context(), outbox.Event{
		Id: "claimed", Status: outbox.StatusPending, LockToken: "lease",
	}))
	write := update.Lookup("updates", "0").Document()
	require.Equal(t, "claimed", write.Lookup("q", collectionFieldId).StringValue())
	require.Equal(t, "lease", write.Lookup("q", collectionFieldLockToken).StringValue())
	set := write.Lookup("u", "0", opSet).Document()
	require.NoError(t, set.Validate())
	require.Equal(t, string(outbox.StatusPending), set.Lookup(collectionFieldStatus).StringValue())
	require.Equal(t, bson.TypeNull, set.Lookup(collectionFieldLockedOn).Type)
	require.Equal(t, bson.TypeNull, set.Lookup(collectionFieldLockToken).Type)
	require.Equal(t, bson.TypeNull, set.Lookup(collectionFieldNextAttemptAt).Type)
	_, err = set.LookupErr(collectionFieldLastAttempts)
	require.Error(t, err)
	_, err = set.LookupErr(collectionFieldLastAttemptOn)
	require.Error(t, err)
}

// A zero batch size must be refused before the store reaches MongoDB, where a
// limit of 0 means no limit; the zero Store has no collection to reach.
func TestFetchUnprocessedEvents_RejectsZeroBatchSize(t *testing.T) {
	t.Parallel()

	events, err := (&Store{}).FetchUnprocessedEvents(t.Context(), 0)
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	require.Empty(t, events)
}

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()
	require.Equal(t, DefaultCollectionName, opts.collectionName)
	require.Equal(t, DefaultIndexCreateTimeout, opts.indexTimeout)
	require.Equal(t, nil, opts.ctx)
}

func TestWithCollectionName_String(t *testing.T) {
	opts := newOptions(WithCollectionName("my_events"))
	require.Equal(t, "my_events", opts.collectionName)
}

func TestWithCollectionName_StringPtr(t *testing.T) {
	name := "ptr_events"
	opts := newOptions(WithCollectionName(&name))
	require.Equal(t, "ptr_events", opts.collectionName)
}

func TestWithCollectionName_NilPtr(t *testing.T) {
	opts := newOptions(WithCollectionName[*string](nil))
	require.Equal(t, DefaultCollectionName, opts.collectionName)
}

func TestWithCollectionName_Trimmed(t *testing.T) {
	opts := newOptions(WithCollectionName("  spaced  "))
	require.Equal(t, "spaced", opts.collectionName)
}

func TestWithContext(t *testing.T) {
	ctx := t.Context()
	opts := newOptions(WithContext(ctx))
	require.Equal(t, ctx, opts.ctx)
}

func TestWithIndexCreateTimeout(t *testing.T) {
	opts := newOptions(WithIndexCreateTimeout(30 * time.Second))
	require.Equal(t, 30*time.Second, opts.indexTimeout)
}

func TestWithIndexCreateTimeout_Negative(t *testing.T) {
	opts := newOptions(WithIndexCreateTimeout(-1))
	require.Equal(t, DefaultIndexCreateTimeout, opts.indexTimeout)
}

func TestWithIndexCreateTimeout_Zero(t *testing.T) {
	opts := newOptions(WithIndexCreateTimeout(0))
	require.Equal(t, DefaultIndexCreateTimeout, opts.indexTimeout)
}

func TestConstants(t *testing.T) {
	require.NotEmpty(t, DefaultCollectionName, "DefaultCollectionName is empty")
	require.True(t, DefaultIndexCreateTimeout > 0, "DefaultIndexCreateTimeout = %v", DefaultIndexCreateTimeout)
}

func TestEventsIndexes(t *testing.T) {
	require.NotEmpty(t, eventsIndexes, "eventsIndexes is empty")
	for i, idx := range eventsIndexes {
		require.NotNil(t, idx.Keys, "index %d has nil Keys", i)
	}
}

func TestCollectionFieldConstants(t *testing.T) {
	fields := []string{
		collectionFieldId,
		collectionFieldPublishedAt,
		collectionFieldStatus,
		collectionFieldLockedOn,
		collectionFieldCreatedAt,
		collectionFieldLastAttemptOn,
		collectionFieldLastAttempts,
		collectionFieldExpiresAt,
		collectionFieldSeq,
	}
	seen := make(map[string]bool)
	for _, f := range fields {
		require.NotEmpty(t, f, "empty field constant")
		require.False(t, seen[f], "duplicate field constant: %q", f)
		seen[f] = true
	}
}
