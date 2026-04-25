// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"context"
	"errors"
	"iter"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/transport/broker"

	corectx "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Option configures the NATS provider.
type Option func(*Nats)

// WithAllowedSubjects restricts Publish and Subscribe to the listed subject
// patterns. NATS wildcards are supported: "*" matches a single token, ">"
// matches one or more trailing tokens. When the allowlist is empty (default),
// all subjects are permitted.
func WithAllowedSubjects(subjects ...string) Option {
	return func(n *Nats) {
		n.allowedSubjects = subjects
	}
}

// WithCollector sets the metrics collector for NATS subscriber instrumentation.
func WithCollector(c metrics.Collector) Option {
	return func(n *Nats) {
		n.collector = c
	}
}

// ErrSubjectNotAllowed is returned by [Nats.Publish], [Nats.PublishBatch],
// and subscriber [Subscribe] when the subject does not match any pattern
// in the allowlist configured via [WithAllowedSubjects].
var ErrSubjectNotAllowed = errors.New("subject not allowed by allowlist")

// Nats implements [broker.Provider] using NATS JetStream.
// It manages a JetStream context derived from the supplied [nats.Conn] and tracks
// active subscribers for bulk unsubscription via [Nats.UnsubscribeAll].
// All exported methods are safe for concurrent use.
type Nats struct {
	natsConn        *nats.Conn
	jetStream       jetstream.JetStream
	subscribersMx   sync.RWMutex
	subscribers     []broker.Subscriber
	allowedSubjects []string
	collector       metrics.Collector
}

// New creates a new NATS JetStream provider.
// Returns error if JetStream is not enabled or context creation fails.
//
// Example:
//
//	provider, err := natsprovider.New(natsConn)
//	b := broker.New(provider)
func New(conn *nats.Conn, opts ...Option) (*Nats, error) {
	//nolint:contextcheck // Convenience wrapper; use NewWithContext when you have an inherited ctx.
	return NewWithContext(context.Background(), conn, opts...)
}

// NewWithContext creates a new NATS JetStream provider using ctx for startup checks.
// It verifies JetStream is enabled on the server by querying account info with a 5-second timeout.
// Returns an error if JetStream is not available or the connection lacks JetStream support.
func NewWithContext(ctx context.Context, conn *nats.Conn, opts ...Option) (*Nats, error) {
	warnIfNoAuth(conn)

	n := &Nats{natsConn: conn}
	for _, o := range opts {
		o(n)
	}

	var err error
	// Attempt to create a new JetStream context.
	n.jetStream, err = jetstream.New(n.natsConn)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create JetStream context")
	}

	ctx, cancel := corectx.ApplyTimeout(ctx, 5*time.Second) //nolint:mnd
	defer cancel()

	// Validate JetStream is enabled on the NATS server.
	_, err = n.jetStream.AccountInfo(ctx)
	if err != nil {
		if errors.Is(err, nats.ErrJetStreamNotEnabled) {
			return nil, errors.New("JetStream is not enabled on the NATS server")
		}
		return nil, coreerrs.WrapOperation(err, "query JetStream account info")
	}

	return n, nil
}

// UnsubscribeAll unsubscribes all active subscribers and clears the internal list.
func (n *Nats) UnsubscribeAll() {
	n.subscribersMx.Lock()
	subs := n.subscribers
	n.subscribers = nil // Clear the list before unsubscribing to avoid holding the lock during callbacks.
	n.subscribersMx.Unlock()

	for _, s := range subs {
		s.Unsubscribe()
	}
}

// Subscribers returns an iterator over active subscribers.
// The iterator provides a snapshot of subscribers at the time of the call,
// so it is safe to use even if subscribers are added or removed concurrently.
//
// Example:
//
//	for sub := range provider.Subscribers() {
//	    // process subscriber
//	}
func (n *Nats) Subscribers() iter.Seq[broker.Subscriber] {
	return func(yield func(broker.Subscriber) bool) {
		n.subscribersMx.RLock()
		subs := slices.Clone(n.subscribers)
		n.subscribersMx.RUnlock()

		for _, sub := range subs {
			if !yield(sub) {
				return
			}
		}
	}
}

// NatsConn returns the underlying NATS connection.
// Useful for advanced use cases that need direct access to the connection.
func (n *Nats) NatsConn() *nats.Conn {
	return n.natsConn
}

// JetStream returns the JetStream context.
// Useful for advanced use cases that need direct JetStream access.
func (n *Nats) JetStream() jetstream.JetStream {
	return n.jetStream
}

// warnIfNoAuth logs a warning when the NATS connection has no authentication
// configured. Running without credentials is a security risk in production.
func warnIfNoAuth(conn *nats.Conn) {
	o := conn.Opts
	hasAuth := o.User != "" ||
		o.Password != "" ||
		o.Token != "" ||
		o.TokenHandler != nil ||
		o.Nkey != "" ||
		o.SignatureCB != nil ||
		o.UserJWT != nil ||
		o.UserInfo != nil
	if !hasAuth {
		slog.Warn("nats: connecting without authentication; consider configuring credentials for production use",
			slog.String("url", conn.ConnectedUrl()))
	}
}

var _ broker.Provider = (*Nats)(nil)
