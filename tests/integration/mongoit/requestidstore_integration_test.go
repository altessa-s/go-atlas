// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-aip/requestid"
	"github.com/altessa-s/go-aip/requestid/requestidtest"

	mongostore "github.com/altessa-s/go-atlas/reliability/requestidstore/storages/mongo"
)

func TestRequestIDStore(t *testing.T) {
	t.Parallel()

	db := client(t).Database("requestid_" + rand.Text())
	t.Cleanup(func() { require.NoError(t, db.Drop(context.WithoutCancel(t.Context()))) })
	requestidtest.Run(t, func(t *testing.T) requestid.Store {
		s, err := mongostore.New(t.Context(), db, mongostore.WithCollectionName(rand.Text()))
		require.NoError(t, err)
		return s
	})
}

func TestRequestIDStoreGarbageCollection(t *testing.T) {
	t.Parallel()

	db := client(t).Database("requestid_" + rand.Text())
	t.Cleanup(func() { require.NoError(t, db.Drop(context.WithoutCancel(t.Context()))) })
	s, err := mongostore.New(t.Context(), db)
	require.NoError(t, err)
	coll := db.Collection(mongostore.DefaultCollectionName)
	indexes, err := coll.Indexes().ListSpecifications(t.Context())
	require.NoError(t, err)
	var ttl bool
	for _, index := range indexes {
		if index.Name == "purge_at_1" && index.ExpireAfterSeconds != nil && *index.ExpireAfterSeconds == 0 {
			ttl = true
		}
	}
	require.True(t, ttl)

	// The caller can be years behind the server. A historical absolute date
	// on the TTL index would make even a fresh lease eligible for deletion.
	now := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	key := requestidtest.Key(t.Name())
	entry := requestid.Entry{State: requestid.StatePending, Token: string([]byte{0xff}), ExpiresAt: now.Add(time.Minute)}
	_, won, err := s.Reserve(t.Context(), key, entry, now)
	require.NoError(t, err)
	require.True(t, won)
	_, won, err = s.Reserve(t.Context(), key, entry, now)
	require.NoError(t, err)
	require.False(t, won)
	entry.State = requestid.StateCompleted
	entry.ExpiresAt = now.Add(time.Hour)
	require.NoError(t, s.Complete(t.Context(), key, entry.Token, entry, now))
	var result struct {
		PurgeAt time.Time `bson:"purge_at"`
	}
	require.NoError(t, coll.FindOne(t.Context(), bson.M{"_id": key}).Decode(&result))
	require.WithinDuration(t, time.Now().Add(time.Hour+time.Minute), result.PurgeAt, 30*time.Second)
}
