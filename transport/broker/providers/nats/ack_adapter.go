// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"errors"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/transport/broker/msg"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ackAdapter implements msg.Acker for NATS JetStream message acknowledgment.
type ackAdapter struct {
	msg jetstream.Msg
}

func (aa *ackAdapter) wrapAckError(action string, err error) error {
	if err != nil && !errors.Is(err, jetstream.ErrMsgAlreadyAckd) {
		return coreerrs.WrapOperation(err, action+" NATS message")
	}
	return nil
}

// Ack sends a positive acknowledgment to NATS JetStream.
// Treats ErrMsgAlreadyAckd as success.
func (aa *ackAdapter) Ack() error {
	return aa.wrapAckError("ack", aa.msg.Ack())
}

// Nak sends a negative acknowledgment for redelivery with optional delay.
// Treats ErrMsgAlreadyAckd as success.
func (aa *ackAdapter) Nak(delay ...time.Duration) error {
	var err error
	if len(delay) > 0 && delay[0] > 0 {
		err = aa.msg.NakWithDelay(delay[0])
	} else {
		err = aa.msg.Nak()
	}
	return aa.wrapAckError("nak", err)
}

// NakWithBackOff sends a negative acknowledgment with a delay computed by backOff
// based on the current delivery attempt count.
// Falls back to instant redelivery if message metadata is unavailable.
// Treats ErrMsgAlreadyAckd as success.
func (aa *ackAdapter) NakWithBackOff(backOff msg.BackOffFunc) error {
	meta, err := aa.msg.Metadata()
	if err != nil {
		return aa.Nak()
	}
	return aa.Nak(backOff(meta.NumDelivered))
}

// Term sends a terminal acknowledgment to prevent redelivery.
// Optional reason can be provided. Treats ErrMsgAlreadyAckd as success.
func (aa *ackAdapter) Term(reason ...string) error {
	var err error
	if len(reason) > 0 && reason[0] != "" {
		err = aa.msg.TermWithReason(reason[0])
	} else {
		err = aa.msg.Term()
	}

	return aa.wrapAckError("term", err)
}

// InProgress sends a heartbeat to prevent premature redelivery.
// Treats ErrMsgAlreadyAckd as success.
func (aa *ackAdapter) InProgress() error {
	return aa.wrapAckError("send in-progress for", aa.msg.InProgress())
}
