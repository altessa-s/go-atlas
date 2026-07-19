// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package panics_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
)

func TestTrySend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(t *testing.T) (chan int, context.Context)
		wantSent   bool
		wantClosed bool
	}{
		{
			name: "open channel with buffer space",
			setup: func(t *testing.T) (chan int, context.Context) {
				t.Helper()
				return make(chan int, 1), t.Context()
			},
			wantSent:   true,
			wantClosed: false,
		},
		{
			name: "closed channel",
			setup: func(t *testing.T) (chan int, context.Context) {
				t.Helper()
				ch := make(chan int, 1)
				close(ch)
				return ch, t.Context()
			},
			wantSent:   false,
			wantClosed: true,
		},
		{
			name: "full buffer with canceled context",
			setup: func(t *testing.T) (chan int, context.Context) {
				t.Helper()
				ch := make(chan int, 1)
				ch <- 1 // fill the buffer so the send blocks
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ch, ctx
			},
			wantSent:   false,
			wantClosed: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ch, ctx := tc.setup(t)
			sent, closed := panics.TrySend(ctx, ch, 42)
			require.Equal(t, tc.wantSent, sent)
			require.Equal(t, tc.wantClosed, closed)
			if sent {
				require.Equal(t, 42, <-ch)
			}
		})
	}
}

func TestTrySend_BlocksUntilReceiverReady(t *testing.T) {
	t.Parallel()

	ch := make(chan int) // unbuffered: the send must block until the receive
	got := make(chan int, 1)
	go func() { got <- <-ch }()

	sent, closed := panics.TrySend(t.Context(), ch, 42)
	require.True(t, sent)
	require.False(t, closed)
	require.Equal(t, 42, <-got)
}

func TestTrySendNonBlocking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(t *testing.T) chan int
		wantSent   bool
		wantClosed bool
	}{
		{
			name: "open channel with buffer space",
			setup: func(t *testing.T) chan int {
				t.Helper()
				return make(chan int, 1)
			},
			wantSent:   true,
			wantClosed: false,
		},
		{
			name: "closed channel",
			setup: func(t *testing.T) chan int {
				t.Helper()
				ch := make(chan int, 1)
				close(ch)
				return ch
			},
			wantSent:   false,
			wantClosed: true,
		},
		{
			name: "full buffer drops the value",
			setup: func(t *testing.T) chan int {
				t.Helper()
				ch := make(chan int, 1)
				ch <- 1
				return ch
			},
			wantSent:   false,
			wantClosed: false,
		},
		{
			name: "nil channel takes the default case",
			setup: func(t *testing.T) chan int {
				t.Helper()
				return nil
			},
			wantSent:   false,
			wantClosed: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ch := tc.setup(t)
			sent, closed := panics.TrySendNonBlocking(ch, 42)
			require.Equal(t, tc.wantSent, sent)
			require.Equal(t, tc.wantClosed, closed)
			if sent {
				require.Equal(t, 42, <-ch)
			}
		})
	}
}
