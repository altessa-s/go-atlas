// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/transport/broker/msg"
)

// stubMsg implements the subset of jetstream.Msg used by ackAdapter.
type stubMsg struct {
	ackErr         error
	nakErr         error
	nakDelayErr    error
	termErr        error
	termReasonErr  error
	inProgressErr  error
	metadataErr    error
	metadata       *jetstream.MsgMetadata
	nakDelayCalled time.Duration
	nakCalled      bool
}

func (s *stubMsg) Ack() error                         { return s.ackErr }
func (s *stubMsg) Nak() error                         { s.nakCalled = true; return s.nakErr }
func (s *stubMsg) NakWithDelay(d time.Duration) error { s.nakDelayCalled = d; return s.nakDelayErr }
func (s *stubMsg) Term() error                        { return s.termErr }
func (s *stubMsg) TermWithReason(r string) error      { return s.termReasonErr }
func (s *stubMsg) InProgress() error                  { return s.inProgressErr }
func (s *stubMsg) Metadata() (*jetstream.MsgMetadata, error) {
	return s.metadata, s.metadataErr
}

// Satisfy the rest of jetstream.Msg interface.
func (s *stubMsg) Data() []byte                      { return nil }
func (s *stubMsg) Subject() string                   { return "" }
func (s *stubMsg) Reply() string                     { return "" }
func (s *stubMsg) Headers() nats.Header              { return nil }
func (s *stubMsg) DoubleAck(_ context.Context) error { return nil }

func TestAckAdapter_NakWithBackOff_UsesDeliveryCount(t *testing.T) {
	stub := &stubMsg{
		metadata: &jetstream.MsgMetadata{NumDelivered: 3},
	}
	aa := &ackAdapter{msg: stub}

	called := false
	var gotAttempt uint64
	backoff := msg.BackOffFunc(func(attempt uint64) time.Duration {
		called = true
		gotAttempt = attempt
		return 42 * time.Second
	})

	if err := aa.NakWithBackOff(backoff); err != nil {
		t.Fatalf("NakWithBackOff() error = %v", err)
	}
	if !called {
		t.Fatal("backoff function was not called")
	}
	if gotAttempt != 3 {
		t.Errorf("backoff called with attempt=%d, want 3", gotAttempt)
	}
	if stub.nakDelayCalled != 42*time.Second {
		t.Errorf("NakWithDelay called with %v, want 42s", stub.nakDelayCalled)
	}
}

func TestAckAdapter_NakWithBackOff_FallsBackOnMetadataError(t *testing.T) {
	stub := &stubMsg{
		metadataErr: errors.New("no metadata"),
	}
	aa := &ackAdapter{msg: stub}

	backoff := msg.BackOffFunc(func(_ uint64) time.Duration {
		t.Fatal("backoff should not be called when metadata fails")
		return 0
	})

	if err := aa.NakWithBackOff(backoff); err != nil {
		t.Fatalf("NakWithBackOff() error = %v", err)
	}
	if !stub.nakCalled {
		t.Fatal("expected plain Nak() to be called on metadata error")
	}
}

func TestAckAdapter_NakWithBackOff_AlreadyAcked(t *testing.T) {
	stub := &stubMsg{
		metadata:    &jetstream.MsgMetadata{NumDelivered: 1},
		nakDelayErr: jetstream.ErrMsgAlreadyAckd,
	}
	aa := &ackAdapter{msg: stub}

	err := aa.NakWithBackOff(func(_ uint64) time.Duration { return time.Second })
	if err != nil {
		t.Fatalf("expected nil for already-acked, got %v", err)
	}
}
