// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"errors"
	"sync"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
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
			if got := tt.s.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRecoveryStrategy_Methods(t *testing.T) {
	if !RecoveryStrategyAuto.IsAuto() {
		t.Fatal("Auto.IsAuto() = false")
	}
	if !RecoveryStrategyManual.IsManual() {
		t.Fatal("Manual.IsManual() = false")
	}
	if !RecoveryStrategySkip.IsSkip() {
		t.Fatal("Skip.IsSkip() = false")
	}
	if RecoveryStrategyAuto.IsManual() {
		t.Fatal("Auto.IsManual() = true")
	}
}

func TestRegistry_StreamOperations(t *testing.T) {
	r := NewRegistry()

	cfg := jetstream.StreamConfig{Name: "test-stream"}
	r.RegisterStream("test-stream", cfg, RecoveryStrategyAuto)

	if !r.HasStream("test-stream") {
		t.Fatal("HasStream() = false")
	}
	if r.HasStream("missing") {
		t.Fatal("HasStream(missing) = true")
	}
	if r.StreamCount() != 1 {
		t.Fatalf("StreamCount() = %d", r.StreamCount())
	}

	got, ok := r.GetStreamConfig("test-stream")
	if !ok {
		t.Fatal("GetStreamConfig() ok = false")
	}
	if got.Name != "test-stream" {
		t.Fatalf("config.Name = %q", got.Name)
	}

	if s := r.GetRecoveryStrategy("test-stream"); !s.IsAuto() {
		t.Fatalf("strategy = %v", s)
	}
	if s := r.GetRecoveryStrategy("missing"); !s.IsAuto() {
		t.Fatal("missing stream should return Auto")
	}
}

func TestRegistry_StreamNames(t *testing.T) {
	r := NewRegistry()
	r.RegisterStream("s1", jetstream.StreamConfig{Name: "s1"}, RecoveryStrategyAuto)
	r.RegisterStream("s2", jetstream.StreamConfig{Name: "s2"}, RecoveryStrategySkip)

	count := 0
	for range r.StreamNames() {
		count++
	}
	if count != 2 {
		t.Fatalf("StreamNames() yielded %d", count)
	}
}

func TestRegistry_UnregisterStream(t *testing.T) {
	r := NewRegistry()
	r.RegisterStream("s1", jetstream.StreamConfig{Name: "s1"}, RecoveryStrategyAuto)
	r.UnregisterStream("s1")
	if r.HasStream("s1") {
		t.Fatal("stream should be unregistered")
	}
}

func TestRegistry_SubscriptionOperations(t *testing.T) {
	r := NewRegistry()
	r.RegisterStream("stream1", jetstream.StreamConfig{Name: "stream1"}, RecoveryStrategyAuto)

	err := r.RegisterSubscription("stream1", "consumer1", nil, nil)
	if err != nil {
		t.Fatalf("RegisterSubscription error = %v", err)
	}

	if !r.HasConsumer("stream1", "consumer1") {
		t.Fatal("HasConsumer() = false")
	}
	if r.HasConsumer("stream1", "missing") {
		t.Fatal("HasConsumer(missing) = true")
	}
	if r.ConsumerCount("stream1") != 1 {
		t.Fatalf("ConsumerCount() = %d", r.ConsumerCount("stream1"))
	}

	count := 0
	for range r.ConsumerNames("stream1") {
		count++
	}
	if count != 1 {
		t.Fatalf("ConsumerNames() yielded %d", count)
	}
}

func TestRegistry_RegisterSubscription_StreamNotRegistered(t *testing.T) {
	r := NewRegistry()
	err := r.RegisterSubscription("missing", "consumer1", nil, nil)
	if !errors.Is(err, ErrStreamNotRegistered) {
		t.Fatalf("error = %v, want ErrStreamNotRegistered", err)
	}
}

func TestRegistry_RegisterSubscription_EmptyConsumer(t *testing.T) {
	r := NewRegistry()
	r.RegisterStream("s1", jetstream.StreamConfig{Name: "s1"}, RecoveryStrategyAuto)
	err := r.RegisterSubscription("s1", "", nil, nil)
	if !errors.Is(err, ErrInvalidConsumerConfig) {
		t.Fatalf("error = %v, want ErrInvalidConsumerConfig", err)
	}
}

func TestRegistry_UnregisterSubscription(t *testing.T) {
	r := NewRegistry()
	r.RegisterStream("s1", jetstream.StreamConfig{Name: "s1"}, RecoveryStrategyAuto)
	r.RegisterSubscription("s1", "c1", nil, nil)
	r.UnregisterSubscription("s1", "c1")
	if r.HasConsumer("s1", "c1") {
		t.Fatal("consumer should be unregistered")
	}
}

func TestRegistry_ResubscribeHandler(t *testing.T) {
	r := NewRegistry()
	called := false
	r.RegisterResubscribeHandler("s1", "c1", func() error {
		called = true
		return nil
	})

	fn, ok := r.GetResubscribeHandler("s1", "c1")
	if !ok {
		t.Fatal("GetResubscribeHandler() ok = false")
	}
	fn()
	if !called {
		t.Fatal("handler not called")
	}

	_, ok = r.GetResubscribeHandler("s1", "missing")
	if ok {
		t.Fatal("should not find missing handler")
	}

	_, ok = r.GetResubscribeHandler("missing", "c1")
	if ok {
		t.Fatal("should not find handler for missing stream")
	}
}

func TestRegistry_GetStreamSubscriptions(t *testing.T) {
	r := NewRegistry()
	r.RegisterStream("s1", jetstream.StreamConfig{Name: "s1"}, RecoveryStrategyAuto)
	r.RegisterSubscription("s1", "c1", nil, nil)
	r.RegisterSubscription("s1", "c2", nil, nil)

	subs := r.GetStreamSubscriptions("s1")
	if len(subs) != 2 {
		t.Fatalf("GetStreamSubscriptions() len = %d, want 2", len(subs))
	}

	subs = r.GetStreamSubscriptions("missing")
	if subs != nil {
		t.Fatal("missing stream should return nil")
	}
}

func TestRegistry_Clear(t *testing.T) {
	r := NewRegistry()
	r.RegisterStream("s1", jetstream.StreamConfig{Name: "s1"}, RecoveryStrategyAuto)
	r.RegisterSubscription("s1", "c1", nil, nil)
	r.Clear()

	if r.StreamCount() != 0 {
		t.Fatalf("StreamCount() = %d after Clear()", r.StreamCount())
	}
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

func TestErrors(t *testing.T) {
	errs := []error{
		ErrStreamNotRegistered, ErrConsumerNotRegistered,
		ErrStreamAlreadyRegistered, ErrRecoveryInProgress,
		ErrMaxRecoveryAttemptsExceeded, ErrManagerClosed,
		ErrNilJetStream, ErrNilNatsConn,
		ErrInvalidStreamConfig, ErrInvalidConsumerConfig,
		ErrSchedulerManaged,
	}

	for _, err := range errs {
		if err == nil {
			t.Fatal("error is nil")
		}
	}
}
