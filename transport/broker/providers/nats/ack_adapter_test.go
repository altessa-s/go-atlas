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
	"github.com/stretchr/testify/require"

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

	err := aa.NakWithBackOff(backoff)
	require.NoError(t, err)
	require.True(t, called, "backoff function was not called")
	require.EqualValues(t, 3, gotAttempt)
	require.Equal(t, 42*time.Second, stub.nakDelayCalled)
}

func TestAckAdapter_NakWithBackOff_FallsBackOnMetadataError(t *testing.T) {
	stub := &stubMsg{
		metadataErr: errors.New("no metadata"),
	}
	aa := &ackAdapter{msg: stub}

	backoff := msg.BackOffFunc(func(_ uint64) time.Duration {
		require.Fail(t, "backoff should not be called when metadata fails")
		return 0
	})

	err := aa.NakWithBackOff(backoff)
	require.NoError(t, err)
	require.True(t, stub.nakCalled, "expected plain Nak() to be called on metadata error")
}

func TestAckAdapter_NakWithBackOff_AlreadyAcked(t *testing.T) {
	stub := &stubMsg{
		metadata:    &jetstream.MsgMetadata{NumDelivered: 1},
		nakDelayErr: jetstream.ErrMsgAlreadyAckd,
	}
	aa := &ackAdapter{msg: stub}

	err := aa.NakWithBackOff(func(_ uint64) time.Duration { return time.Second })
	require.NoError(t, err)
}
