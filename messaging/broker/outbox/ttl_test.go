// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/messaging/broker/msg"
	"github.com/altessa-s/go-atlas/messaging/broker/outbox"

	dataoutbox "github.com/altessa-s/go-atlas/reliability/outbox"
)

func TestPublish_RelayedMessageGetsFreshEventTTL(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const ttl = time.Hour
		store := &oneShotStore{}
		pub := &capturingPublisher{}
		ob := outbox.New(store, pub, outbox.WithOutboxOptions(dataoutbox.WithDefaultEventTTL(ttl)))
		saveTime := time.Now().UTC()
		originalTime := saveTime.Add(-2 * ttl).Format(msg.MessageCreatedTimeFormat)
		message := msg.NewMessageWithMeta("billing.invoice.paid", nil, msg.Meta{{Key: msg.MetaKeyMessageCreatedTime, Value: originalTime}})

		require.NoError(t, ob.Publish(t.Context(), *message))
		require.Len(t, store.saved, 1)
		require.Equal(t, saveTime, store.saved[0].CreatedAt)
		require.Equal(t, saveTime.Add(ttl), store.saved[0].ExpiresAt)

		store.fetch = store.saved
		require.NoError(t, ob.RunDispatchCycle(t.Context()))
		require.Len(t, pub.published(), 1)
		createdTime, ok := pub.published()[0].Metadata.Value(msg.MetaKeyMessageCreatedTime)
		require.True(t, ok)
		require.Equal(t, originalTime, createdTime)
	})
}
