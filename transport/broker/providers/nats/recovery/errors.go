// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import "errors"

var (
	// ErrStreamNotRegistered is returned by [Manager.Subscribe] and
	// [Registry.RegisterSubscription] when the stream has not been
	// registered via [Manager.RegisterStream].
	ErrStreamNotRegistered = errors.New("stream not registered")

	// ErrConsumerNotRegistered is returned when attempting to operate on a
	// consumer that has not been registered with the [Manager].
	ErrConsumerNotRegistered = errors.New("consumer not registered")

	// ErrStreamAlreadyRegistered is returned when attempting to register a
	// stream that is already registered.
	ErrStreamAlreadyRegistered = errors.New("stream already registered")

	// ErrRecoveryInProgress is returned when a recovery operation is already
	// in progress for the specified stream.
	ErrRecoveryInProgress = errors.New("recovery already in progress")

	// ErrMaxRecoveryAttemptsExceeded is returned when the maximum number of
	// recovery attempts (configured via [WithMaxRecoveryAttempts]) has been
	// exceeded.
	ErrMaxRecoveryAttemptsExceeded = errors.New("max recovery attempts exceeded")

	// ErrManagerClosed is returned by [Manager.Start], [Manager.Subscribe],
	// [Manager.RegisterStream], and [Manager.CreateStream] when the
	// [Manager] has been closed via [Manager.Close].
	ErrManagerClosed = errors.New("manager is closed")

	// ErrNilJetStream is returned by [New] when a nil NATS provider is passed.
	ErrNilJetStream = errors.New("jetstream instance is required")

	// ErrNilNatsConn is returned when a nil NATS connection is provided.
	ErrNilNatsConn = errors.New("nats connection is required")

	// ErrInvalidStreamConfig is returned by [Manager.RegisterStream] and
	// [Manager.CreateStream] when the stream configuration has an empty name.
	ErrInvalidStreamConfig = errors.New("invalid stream configuration")

	// ErrInvalidConsumerConfig is returned by [Manager.Subscribe] and
	// [Registry.RegisterSubscription] when the consumer name is empty.
	ErrInvalidConsumerConfig = errors.New("invalid consumer configuration")
)
