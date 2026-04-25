// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"errors"
	"sync"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

func TestRecoveryStrategy_String(t *testing.T) {
	tests := []struct {
		s    RecoveryStrategy
		want string
	}{
		{RecoveryStrategyAuto, "auto"},
		{RecoveryStrategyManual, "manual"},
		{RecoveryStrategySkip, "skip"},
		{RecoveryStrategy(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.s.String()
			require.Equal(t, tt.want, got)
		})
	}
}

func TestRecoveryStrategy_Methods(t *testing.T) {
	require.True(t, RecoveryStrategyAuto.IsAuto(), "Auto.IsAuto() = false")
	require.True(t, RecoveryStrategyManual.IsManual(), "Manual.IsManual() = false")
	require.True(t, RecoveryStrategySkip.IsSkip(), "Skip.IsSkip() = false")
	require.False(t, RecoveryStrategyAuto.IsManual(), "Auto.IsManual() = true")
}

func TestRegistry_StreamOperations(t *testing.T) {
	r := NewRegistry()

	cfg := jetstream.StreamConfig{Name: "test-stream"}
	r.RegisterStream("test-stream", cfg, RecoveryStrategyAuto)

	require.True(t, r.HasStream("test-stream"), "HasStream() = false")
	require.False(t, r.HasStream("missing"), "HasStream(missing) = true")
	require.Equal(t, 1, r.StreamCount())

	got, ok := r.GetStreamConfig("test-stream")
	require.True(t, ok, "GetStreamConfig() ok = false")
	require.Equal(t, "test-stream", got.Name)

	s := r.GetRecoveryStrategy("test-stream")
	require.True(t, s.IsAuto(), "strategy = %v", s)
	s = r.GetRecoveryStrategy("missing")
	require.True(t, s.IsAuto(), "missing stream should return Auto")
}

func TestRegistry_StreamNames(t *testing.T) {
	r := NewRegistry()
	r.RegisterStream("s1", jetstream.StreamConfig{Name: "s1"}, RecoveryStrategyAuto)
	r.RegisterStream("s2", jetstream.StreamConfig{Name: "s2"}, RecoveryStrategySkip)

	count := 0
	for range r.StreamNames() {
		count++
	}
	require.Equal(t, 2, count)
}

func TestRegistry_UnregisterStream(t *testing.T) {
	r := NewRegistry()
	r.RegisterStream("s1", jetstream.StreamConfig{Name: "s1"}, RecoveryStrategyAuto)
	r.UnregisterStream("s1")
	require.False(t, r.HasStream("s1"), "stream should be unregistered")
}

func TestRegistry_SubscriptionOperations(t *testing.T) {
	r := NewRegistry()
	r.RegisterStream("stream1", jetstream.StreamConfig{Name: "stream1"}, RecoveryStrategyAuto)

	err := r.RegisterSubscription("stream1", "consumer1", nil, nil)
	require.NoError(t, err)

	require.True(t, r.HasConsumer("stream1", "consumer1"), "HasConsumer() = false")
	require.False(t, r.HasConsumer("stream1", "missing"), "HasConsumer(missing) = true")
	require.Equal(t, 1, r.ConsumerCount("stream1"))

	count := 0
	for range r.ConsumerNames("stream1") {
		count++
	}
	require.Equal(t, 1, count)
}

func TestRegistry_RegisterSubscription_StreamNotRegistered(t *testing.T) {
	r := NewRegistry()
	err := r.RegisterSubscription("missing", "consumer1", nil, nil)
	require.True(t, errors.Is(err, ErrStreamNotRegistered), "error = %v, want ErrStreamNotRegistered", err)
}

func TestRegistry_RegisterSubscription_EmptyConsumer(t *testing.T) {
	r := NewRegistry()
	r.RegisterStream("s1", jetstream.StreamConfig{Name: "s1"}, RecoveryStrategyAuto)
	err := r.RegisterSubscription("s1", "", nil, nil)
	require.True(t, errors.Is(err, ErrInvalidConsumerConfig), "error = %v, want ErrInvalidConsumerConfig", err)
}

func TestRegistry_UnregisterSubscription(t *testing.T) {
	r := NewRegistry()
	r.RegisterStream("s1", jetstream.StreamConfig{Name: "s1"}, RecoveryStrategyAuto)
	r.RegisterSubscription("s1", "c1", nil, nil)
	r.UnregisterSubscription("s1", "c1")
	require.False(t, r.HasConsumer("s1", "c1"), "consumer should be unregistered")
}

func TestRegistry_ResubscribeHandler(t *testing.T) {
	r := NewRegistry()
	called := false
	r.RegisterResubscribeHandler("s1", "c1", func() error {
		called = true
		return nil
	})

	fn, ok := r.GetResubscribeHandler("s1", "c1")
	require.True(t, ok, "GetResubscribeHandler() ok = false")
	fn()
	require.True(t, called, "handler not called")

	_, ok = r.GetResubscribeHandler("s1", "missing")
	require.False(t, ok, "should not find missing handler")

	_, ok = r.GetResubscribeHandler("missing", "c1")
	require.False(t, ok, "should not find handler for missing stream")
}

func TestRegistry_GetStreamSubscriptions(t *testing.T) {
	r := NewRegistry()
	r.RegisterStream("s1", jetstream.StreamConfig{Name: "s1"}, RecoveryStrategyAuto)
	r.RegisterSubscription("s1", "c1", nil, nil)
	r.RegisterSubscription("s1", "c2", nil, nil)

	subs := r.GetStreamSubscriptions("s1")
	require.Len(t, subs, 2)

	subs = r.GetStreamSubscriptions("missing")
	require.Nil(t, subs)
}

func TestRegistry_Clear(t *testing.T) {
	r := NewRegistry()
	r.RegisterStream("s1", jetstream.StreamConfig{Name: "s1"}, RecoveryStrategyAuto)
	r.RegisterSubscription("s1", "c1", nil, nil)
	r.Clear()

	require.Equal(t, 0, r.StreamCount())
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	r.RegisterStream("s1", jetstream.StreamConfig{Name: "s1"}, RecoveryStrategyAuto)

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			r.HasStream("s1")
			r.StreamCount()
			r.GetRecoveryStrategy("s1")
		})
	}
	wg.Wait()
}
