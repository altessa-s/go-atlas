// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/altessa-s/go-atlas/data/idempotency"
	memorystorage "github.com/altessa-s/go-atlas/data/idempotency/storages/memory"
	fieldbehaviortestv1 "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
	brokeridem "github.com/altessa-s/go-atlas/transport/broker/idempotency"
	"github.com/altessa-s/go-atlas/transport/broker/msg"
)

type mockAcker struct {
	mu           sync.Mutex
	acked        bool
	naked        bool
	nakDelay     time.Duration
	nakBackOff   bool
	termed       bool
	termReason   string
	inProgressed bool
}

func (m *mockAcker) Ack() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.acked = true
	return nil
}

func (m *mockAcker) Nak(delay ...time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.naked = true
	if len(delay) > 0 {
		m.nakDelay = delay[0]
	}
	return nil
}

func (m *mockAcker) NakWithBackOff(_ msg.BackOffFunc) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.naked = true
	m.nakBackOff = true
	return nil
}

func (m *mockAcker) Term(reason ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.termed = true
	if len(reason) > 0 {
		m.termReason = reason[0]
	}
	return nil
}

func (m *mockAcker) InProgress() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inProgressed = true
	return nil
}

type testSubscriberHandler struct {
	topic string
	fn    func(ctx context.Context, m *msg.Message)
}

func (h *testSubscriberHandler) Topic() string {
	return h.topic
}

func (h *testSubscriberHandler) Handle(ctx context.Context, m *msg.Message) {
	if h.fn != nil {
		h.fn(ctx, m)
	}
}

func newTestKeeper(t testing.TB) (*idempotency.Keeper, *memorystorage.Storage) {
	t.Helper()
	store := memorystorage.New()
	keeper := idempotency.New(store, idempotency.WithMaxLockDuration(time.Minute))
	return keeper, store
}

func TestWrap_PanicsOnInvalidArgs(t *testing.T) {
	t.Parallel()
	keeper, _ := newTestKeeper(t)
	handler := &testSubscriberHandler{topic: "test"}

	require.PanicsWithValue(t, "idempotency: handler cannot be nil", func() {
		brokeridem.Wrap(nil, keeper, "consumer")
	})

	require.PanicsWithValue(t, "idempotency: keeper cannot be nil", func() {
		brokeridem.Wrap(handler, nil, "consumer")
	})

	require.PanicsWithValue(t, "idempotency: consumer cannot be empty", func() {
		brokeridem.Wrap(handler, keeper, "")
	})
}

func TestWrap_DefaultKeyFunc(t *testing.T) {
	t.Parallel()

	// Nil message
	k, err := brokeridem.DefaultKeyFunc(nil)
	require.NoError(t, err)
	require.Empty(t, k)

	// DeduplicateId priority
	m1 := msg.NewMessageWithMeta("t", []byte("data"), msg.Meta{
		{Key: msg.MetaKeyDeduplicateId, Value: "dedup-123"},
		{Key: msg.MetaKeyMessageId, Value: "msg-456"},
	})
	k1, err := brokeridem.DefaultKeyFunc(m1)
	require.NoError(t, err)
	require.Equal(t, "dedup-123", k1)

	// MessageId fallback
	m2 := msg.NewMessageWithMeta("t", []byte("data"), msg.Meta{
		{Key: msg.MetaKeyMessageId, Value: "msg-456"},
	})
	k2, err := brokeridem.DefaultKeyFunc(m2)
	require.NoError(t, err)
	require.Equal(t, "msg-456", k2)

	// Neither
	m3 := msg.NewMessage("t", []byte("data"))
	k3, err := brokeridem.DefaultKeyFunc(m3)
	require.NoError(t, err)
	require.Empty(t, k3)
}

func TestWrap_ProtoKeyFunc(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() {
		brokeridem.ProtoKeyFunc[*fieldbehaviortestv1.Resource](nil, (*fieldbehaviortestv1.Resource).GetId)
	})

	keyFn := brokeridem.ProtoKeyFunc(
		func() *fieldbehaviortestv1.Resource { return new(fieldbehaviortestv1.Resource) },
		(*fieldbehaviortestv1.Resource).GetId,
	)

	// Nil message
	k, err := keyFn(nil)
	require.NoError(t, err)
	require.Empty(t, k)

	// Valid payload
	validRes := &fieldbehaviortestv1.Resource{Id: "proto-id-999"}
	validBytes, err := proto.Marshal(validRes)
	require.NoError(t, err)
	mValid := msg.NewMessage("t", validBytes)
	kValid, err := keyFn(mValid)
	require.NoError(t, err)
	require.Equal(t, "proto-id-999", kValid)

	// Invalid payload returns error
	mCorrupt := msg.NewMessage("t", []byte("invalid protobuf binary payload"))
	kCorrupt, err := keyFn(mCorrupt)
	require.Error(t, err)
	require.Empty(t, kCorrupt)
}

func TestWrap_DeliveryFlows(t *testing.T) {
	t.Parallel()

	t.Run("successful processing completes lock and acks", func(t *testing.T) {
		t.Parallel()
		keeper, _ := newTestKeeper(t)
		acker := &mockAcker{}
		m := msg.NewMessageWithMeta("topic", []byte("body"), msg.Meta{
			{Key: msg.MetaKeyDeduplicateId, Value: "key-1"},
		}, msg.WithAcker(acker))

		var handlerCalled bool
		h := &testSubscriberHandler{
			topic: "topic",
			fn: func(_ context.Context, in *msg.Message) {
				handlerCalled = true
				require.NoError(t, in.Ack())
			},
		}

		wrapped := brokeridem.Wrap(h, keeper, "consumer_a")
		require.Equal(t, "topic", wrapped.Topic())

		wrapped.Handle(t.Context(), m)
		require.True(t, handlerCalled)
		require.True(t, acker.acked)

		// Second delivery for the same key must be detected as duplicate and Acked without handler
		handlerCalled2 := false
		acker2 := &mockAcker{}
		m2 := msg.NewMessageWithMeta("topic", []byte("body"), msg.Meta{
			{Key: msg.MetaKeyDeduplicateId, Value: "key-1"},
		}, msg.WithAcker(acker2))

		h.fn = func(_ context.Context, _ *msg.Message) {
			handlerCalled2 = true
		}

		wrapped.Handle(t.Context(), m2)
		require.False(t, handlerCalled2, "handler must not be called on duplicate")
		require.True(t, acker2.acked, "duplicate message must be acked")
	})

	t.Run("busy lock sends NakWithBackOff", func(t *testing.T) {
		t.Parallel()
		keeper, _ := newTestKeeper(t)

		// Acquire lock outside to simulate in-progress processing by another worker
		storageKey := brokeridem.BuildStorageKey("consumer_b", "key-busy")
		acquired, _, err := keeper.AttemptLock(t.Context(), storageKey)
		require.NoError(t, err)
		require.True(t, acquired)

		acker := &mockAcker{}
		m := msg.NewMessageWithMeta("topic", []byte("body"), msg.Meta{
			{Key: msg.MetaKeyDeduplicateId, Value: "key-busy"},
		}, msg.WithAcker(acker))

		var handlerCalled bool
		h := &testSubscriberHandler{
			topic: "topic",
			fn: func(_ context.Context, _ *msg.Message) {
				handlerCalled = true
			},
		}

		wrapped := brokeridem.Wrap(h, keeper, "consumer_b")
		wrapped.Handle(t.Context(), m)

		require.False(t, handlerCalled)
		require.True(t, acker.naked)
		require.True(t, acker.nakBackOff)
	})

	t.Run("empty key passes through without lock", func(t *testing.T) {
		t.Parallel()
		keeper, _ := newTestKeeper(t)
		acker := &mockAcker{}
		m := msg.NewMessage("topic", []byte("body"), msg.WithAcker(acker))

		var handlerCalled bool
		h := &testSubscriberHandler{
			topic: "topic",
			fn: func(_ context.Context, in *msg.Message) {
				handlerCalled = true
				require.NoError(t, in.Ack())
			},
		}

		wrapped := brokeridem.Wrap(h, keeper, "consumer_c")
		wrapped.Handle(t.Context(), m)

		require.True(t, handlerCalled)
		require.True(t, acker.acked)
	})

	t.Run("key function error causes Term", func(t *testing.T) {
		t.Parallel()
		keeper, _ := newTestKeeper(t)
		acker := &mockAcker{}
		m := msg.NewMessage("topic", []byte("invalid-proto"), msg.WithAcker(acker))

		var handlerCalled bool
		h := &testSubscriberHandler{
			topic: "topic",
			fn: func(_ context.Context, _ *msg.Message) {
				handlerCalled = true
			},
		}

		wrapped := brokeridem.Wrap(h, keeper, "consumer_d",
			brokeridem.WithKeyFunc(func(_ *msg.Message) (string, error) {
				return "", errors.New("cannot decode key")
			}),
		)
		wrapped.Handle(t.Context(), m)

		require.False(t, handlerCalled)
		require.True(t, acker.termed)
		require.Contains(t, acker.termReason, "cannot decode key")
	})

	t.Run("nak releases lock for redelivery", func(t *testing.T) {
		t.Parallel()
		keeper, _ := newTestKeeper(t)
		acker := &mockAcker{}
		m := msg.NewMessageWithMeta("topic", []byte("body"), msg.Meta{
			{Key: msg.MetaKeyDeduplicateId, Value: "key-nak"},
		}, msg.WithAcker(acker))

		h := &testSubscriberHandler{
			topic: "topic",
			fn: func(_ context.Context, in *msg.Message) {
				require.NoError(t, in.Nak(5*time.Second))
			},
		}

		wrapped := brokeridem.Wrap(h, keeper, "consumer_e")
		wrapped.Handle(t.Context(), m)

		require.True(t, acker.naked)
		require.Equal(t, 5*time.Second, acker.nakDelay)

		// Storage lock must be released, allowing another delivery attempt to acquire
		storageKey := brokeridem.BuildStorageKey("consumer_e", "key-nak")
		acquired, _, err := keeper.AttemptLock(t.Context(), storageKey)
		require.NoError(t, err)
		require.True(t, acquired, "key must be re-acquirable after Nak release")
	})

	t.Run("term releases lock", func(t *testing.T) {
		t.Parallel()
		keeper, _ := newTestKeeper(t)
		acker := &mockAcker{}
		m := msg.NewMessageWithMeta("topic", []byte("body"), msg.Meta{
			{Key: msg.MetaKeyDeduplicateId, Value: "key-term"},
		}, msg.WithAcker(acker))

		h := &testSubscriberHandler{
			topic: "topic",
			fn: func(_ context.Context, in *msg.Message) {
				require.NoError(t, in.Term("bad message"))
			},
		}

		wrapped := brokeridem.Wrap(h, keeper, "consumer_f")
		wrapped.Handle(t.Context(), m)

		require.True(t, acker.termed)
		require.Equal(t, "bad message", acker.termReason)

		// Storage lock must be released
		storageKey := brokeridem.BuildStorageKey("consumer_f", "key-term")
		acquired, _, err := keeper.AttemptLock(t.Context(), storageKey)
		require.NoError(t, err)
		require.True(t, acquired, "key must be re-acquirable after Term release")
	})

	t.Run("in_progress delegates transparently", func(t *testing.T) {
		t.Parallel()
		keeper, _ := newTestKeeper(t)
		acker := &mockAcker{}
		m := msg.NewMessageWithMeta("topic", []byte("body"), msg.Meta{
			{Key: msg.MetaKeyDeduplicateId, Value: "key-prog"},
		}, msg.WithAcker(acker))

		h := &testSubscriberHandler{
			topic: "topic",
			fn: func(_ context.Context, in *msg.Message) {
				require.NoError(t, in.InProgress())
				require.NoError(t, in.Ack())
			},
		}

		wrapped := brokeridem.Wrap(h, keeper, "consumer_g")
		wrapped.Handle(t.Context(), m)

		require.True(t, acker.inProgressed)
		require.True(t, acker.acked)
	})
}

func TestWrap_OnceSemantics(t *testing.T) {
	t.Parallel()
	keeper, _ := newTestKeeper(t)
	acker := &mockAcker{}
	m := msg.NewMessageWithMeta("topic", []byte("body"), msg.Meta{
		{Key: msg.MetaKeyDeduplicateId, Value: "key-once"},
	}, msg.WithAcker(acker))

	h := &testSubscriberHandler{
		topic: "topic",
		fn: func(_ context.Context, in *msg.Message) {
			require.NoError(t, in.Ack())
			// Calling Ack a second time or Nak afterwards should not double-complete or fail
			require.NoError(t, in.Ack())
			require.NoError(t, in.Nak())
		},
	}

	wrapped := brokeridem.Wrap(h, keeper, "consumer_once")
	wrapped.Handle(t.Context(), m)
	require.True(t, acker.acked)
}

func TestWrap_PanicReleasesLock(t *testing.T) {
	t.Parallel()
	keeper, _ := newTestKeeper(t)
	acker := &mockAcker{}
	m := msg.NewMessageWithMeta("topic", []byte("body"), msg.Meta{
		{Key: msg.MetaKeyDeduplicateId, Value: "key-panic"},
	}, msg.WithAcker(acker))

	h := &testSubscriberHandler{
		topic: "topic",
		fn: func(_ context.Context, _ *msg.Message) {
			panic("unexpected handler boom")
		},
	}

	wrapped := brokeridem.Wrap(h, keeper, "consumer_panic")

	require.PanicsWithValue(t, "unexpected handler boom", func() {
		wrapped.Handle(t.Context(), m)
	})

	// Lock must be released despite the panic
	storageKey := brokeridem.BuildStorageKey("consumer_panic", "key-panic")
	acquired, _, err := keeper.AttemptLock(t.Context(), storageKey)
	require.NoError(t, err)
	require.True(t, acquired, "key must be re-acquirable after panic release")
}

func TestWrap_CanceledContextOnAck(t *testing.T) {
	t.Parallel()
	keeper, _ := newTestKeeper(t)
	acker := &mockAcker{}
	m := msg.NewMessageWithMeta("topic", []byte("body"), msg.Meta{
		{Key: msg.MetaKeyDeduplicateId, Value: "key-cancel"},
	}, msg.WithAcker(acker))

	ctx, cancel := context.WithCancel(t.Context())

	h := &testSubscriberHandler{
		topic: "topic",
		fn: func(_ context.Context, in *msg.Message) {
			cancel() // cancel context before calling Ack
			require.NoError(t, in.Ack())
		},
	}

	wrapped := brokeridem.Wrap(h, keeper, "consumer_cancel")
	wrapped.Handle(ctx, m)

	require.True(t, acker.acked)

	// State should still be marked SUCCESS
	storageKey := brokeridem.BuildStorageKey("consumer_cancel", "key-cancel")
	acquired, state, err := keeper.AttemptLock(t.Context(), storageKey)
	require.NoError(t, err)
	require.False(t, acquired)
	require.NotNil(t, state)
	require.Equal(t, idempotency.StatusSuccess, state.Status)
}

func TestWrap_ConcurrentDeliveries(t *testing.T) {
	t.Parallel()
	keeper, _ := newTestKeeper(t)
	wrapped := brokeridem.Wrap(&testSubscriberHandler{
		topic: "topic",
		fn: func(_ context.Context, in *msg.Message) {
			time.Sleep(10 * time.Millisecond)
			_ = in.Ack()
		},
	}, keeper, "consumer_concurrent")

	var (
		wg          sync.WaitGroup
		processed   atomic.Int64
		concurrency = 20
	)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			acker := &mockAcker{}
			m := msg.NewMessageWithMeta("topic", []byte("body"), msg.Meta{
				{Key: msg.MetaKeyDeduplicateId, Value: "same-key"},
			}, msg.WithAcker(acker))

			wrapped.Handle(t.Context(), m)

			acker.mu.Lock()
			if acker.acked {
				processed.Add(1)
			}
			acker.mu.Unlock()
		}()
	}

	wg.Wait()
	require.Positive(t, processed.Load())
}
