// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestDeduplicateBypassesSessions pins that a read bound to a session never
// joins, nor is joined by, another caller's in-flight read: the session-bound
// call runs its own function while an identical flight is still blocked.
func TestDeduplicateBypassesSessions(t *testing.T) {
	t.Parallel()

	m, err := New("db")
	require.NoError(t, err)

	client, err := mongo.Connect(mongoOptions.Client().ApplyURI("mongodb://127.0.0.1:1"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	sess, err := client.StartSession()
	require.NoError(t, err)
	t.Cleanup(func() { sess.EndSession(context.Background()) })
	sessCtx := mongo.NewSessionContext(t.Context(), sess)

	const key = "db:coll:filter"
	release := make(chan struct{})
	entered := make(chan struct{})
	go func() {
		_, _ = m.deduplicate(t.Context(), key, func() (any, error) {
			close(entered)
			<-release
			return "outside", nil
		})
	}()
	<-entered
	defer close(release)

	done := make(chan any, 1)
	go func() {
		v, _ := m.deduplicate(sessCtx, key, func() (any, error) { return "inside", nil })
		done <- v
	}()
	select {
	case v := <-done:
		require.Equal(t, "inside", v, "the session-bound read ran on its own")
	case <-time.After(5 * time.Second):
		t.Fatal("the session-bound read joined the other caller's flight")
	}
}
