// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestDeduplicateBypassesSessions checks that a session-bound read issues its
// own query while an identical read is blocked.
func TestDeduplicateBypassesSessions(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		outside := cursorBatchReply("db.items", bson.D{{Key: "name", Value: "outside"}})
		inside := cursorBatchReply("db.items", bson.D{{Key: "name", Value: "inside"}})
		m, db, entered, release, finds := gatedRepository(t, true, outside, inside)
		unblock := sync.OnceFunc(func() { close(release) })
		defer unblock()
		coll := db.Collection("items")
		sess, err := db.Client().StartSession()
		require.NoError(t, err)
		defer sess.EndSession(t.Context())

		read := func(ctx context.Context) (flightModel, error) {
			return GetEntity[flightModel, flightModel](ctx, m, coll, bson.D{}, FilterEncoding{Registry: bson.NewRegistry()})
		}
		a := beginRead(func() (flightModel, error) { return read(t.Context()) })
		<-entered
		b := beginRead(func() (flightModel, error) { return read(mongo.NewSessionContext(t.Context(), sess)) })
		synctest.Wait()
		require.Len(t, b, 1, "the session read must complete while the other query is gated")
		require.Equal(t, "inside", readValue(t, b).Name)
		unblock()
		require.Equal(t, "outside", readValue(t, a).Name)
		require.EqualValues(t, 2, finds.Load())
	})
}
