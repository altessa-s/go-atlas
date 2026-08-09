// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// The pipeline is the contract with the server: matching anything but inserts
// would wake the dispatcher on its own status writes, and dropping the
// projection would ship a full outbox payload per change event.
func TestWatchPipeline(t *testing.T) {
	t.Parallel()

	require.Len(t, watchPipeline, 2)
	require.Len(t, watchPipeline[0], 1)
	require.Len(t, watchPipeline[1], 1)

	require.Equal(t, "$match", watchPipeline[0][0].Key)
	match, ok := watchPipeline[0][0].Value.(bson.M)
	require.True(t, ok, "$match value must be a document")
	require.Equal(t, operationTypeInsert, match[changeStreamFieldOperationType])

	require.Equal(t, "$project", watchPipeline[1][0].Key)
	project, ok := watchPipeline[1][0].Value.(bson.M)
	require.True(t, ok, "$project value must be a document")
	require.Len(t, project, 1, "the resume token is the only field the watcher reads")
	require.Equal(t, 1, project[collectionFieldId])
}

func TestIsServerErrorCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		code int
		want bool
	}{
		{name: "nil error", err: nil, code: changeStreamHistoryLost},
		{name: "plain error", err: errors.New("boom"), code: changeStreamHistoryLost},
		{
			name: "matching code",
			err:  mongo.CommandError{Code: changeStreamHistoryLost},
			code: changeStreamHistoryLost,
			want: true,
		},
		{
			name: "other code",
			err:  mongo.CommandError{Code: commandNotFound},
			code: changeStreamHistoryLost,
		},
		{
			name: "wrapped matching code",
			err:  errors.Join(errors.New("context"), mongo.CommandError{Code: changeStreamNotSupported}),
			code: changeStreamNotSupported,
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, isServerErrorCode(tc.err, tc.code))
		})
	}
}

func TestSleep(t *testing.T) {
	t.Parallel()

	t.Run("elapsed timer", func(t *testing.T) {
		t.Parallel()
		require.True(t, sleep(t.Context(), time.Millisecond))
	})

	t.Run("cancellation wins", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.False(t, sleep(ctx, time.Hour),
			"a canceled context must end the reconnect loop instead of waiting out the backoff")
	})

	t.Run("non-positive delay", func(t *testing.T) {
		t.Parallel()
		require.True(t, sleep(t.Context(), 0), "a zero delay on a live context is a no-op success")
	})

	t.Run("non-positive delay on a dead context", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.False(t, sleep(ctx, 0))
	})
}

func TestWatchConstants(t *testing.T) {
	t.Parallel()

	require.Less(t, watchRetryBaseDelay, watchRetryMaxDelay,
		"the backoff must have room to grow")
	require.Positive(t, watchMaxAwaitTime,
		"a non-positive await time would make the server return immediately, busy-looping getMore")
	require.Positive(t, watchCloseTimeout,
		"closing on an already-expired context leaks the server-side cursor")
}
