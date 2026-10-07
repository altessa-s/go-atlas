// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package driver

import (
	"context"
)

// Driver defines lifecycle hooks for the driven interceptor pattern.
// Implementations receive callbacks at specific points during RPC execution.
// It is designed to be request-scoped.
//
// Example implementation:
//
//	type myDriver struct{}
//	func (d *myDriver) PreCall(ctx context.Context, req any) (any, error) {
//	    return nil, nil // proceed to handler
//	}
//	func (d *myDriver) PostCall(ctx context.Context, resp any, err error) error {
//	    return err // pass through error
//	}
type Driver interface {
	// PreCall is called before the RPC handler execution.
	// If the returned response is not nil, it will be sent to the client
	// and the actual handler will be skipped.
	// This applies to server interceptors and for unary RPCs only.
	PreCall(ctx context.Context, req any) (any, error)
	// PostCall is called after the RPC handler execution.
	// It can modify the error returned to the client.
	PostCall(ctx context.Context, resp any, err error) error
}

// DriverStream extends Driver for streaming RPCs.
// It provides hooks for individual messages sent or received in a stream.
type DriverStream interface {
	// PostMsgReceive is called after a message is received from the peer.
	PostMsgReceive(ctx context.Context, req any, err error) error
	// PostMsgSent is called after a message is sent to the peer.
	PostMsgSent(ctx context.Context, resp any, err error) error
}

// DriverStreamPreSend is an optional streaming hook called before a message is
// handed to the transport. It returns the message to send, which may be a
// transformed copy, so a driver can sanitize an outgoing message without
// mutating the caller's. A non-nil error aborts the send: the message is not
// sent and the post-send hook of the same wrapper is not called.
//
// On a server, pre-send hooks of nested stream wrappers run from the
// innermost interceptor (closest to the handler) outwards and post-send hooks
// in the reverse order. On a client, wrappers are applied as the streamer
// returns, so pre-send hooks run from the outermost interceptor inwards and
// post-send hooks in the reverse order.
type DriverStreamPreSend interface {
	PreMsgSend(ctx context.Context, m any) (any, error)
}

// DrivenInterceptor provides metadata access and lifecycle hooks via Driver.
// It is the entry point for implementing the Driven Interceptor pattern.
//
// Example:
//
//	type myInterceptor struct{}
//	func (i *myInterceptor) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
//	    return &myDriver{}, ctx
//	}
type DrivenInterceptor interface {
	// DrivenInterceptor returns a request-scoped Driver and a modified context.
	DrivenInterceptor(ctx context.Context) (Driver, context.Context)
}

// StreamType identifies the streaming direction of a gRPC call.
type StreamType int

const (
	// StreamTypeNone indicates a unary RPC.
	StreamTypeNone StreamType = iota
	// StreamTypeClient indicates client-side streaming.
	StreamTypeClient
	// StreamTypeServer indicates server-side streaming.
	StreamTypeServer
	// StreamTypeBidi indicates bidirectional streaming.
	StreamTypeBidi
)

// String returns the string representation of StreamType.
func (s StreamType) String() string {
	switch s {
	case StreamTypeNone:
		return "none"
	case StreamTypeClient:
		return "client"
	case StreamTypeServer:
		return "server"
	case StreamTypeBidi:
		return "bidi"
	default:
		return "unknown"
	}
}

// NoopDriver returns a no-op Driver.
// Useful for tests or when an interceptor should do nothing for a specific call.
func NoopDriver() Driver {
	return noopDriver{}
}

type noopDriver struct{}

func (noopDriver) PreCall(context.Context, any) (any, error) { return nil, nil } //nolint:nilnil // NoopDriver intentionally returns nil values

// PostCall passes the error through unchanged. Returning a hard-coded nil here
// would cause the no-op driver to silently swallow upstream errors when used
// as part of an interceptor chain (for example, when an interceptor without
// custom logic relies on NoopDriver for its lifecycle hooks).
func (noopDriver) PostCall(_ context.Context, _ any, err error) error { return err }
