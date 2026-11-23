// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import "context"

// Handler is a function that dispatches an event to an external system.
// It is called by the Outbox during the dispatch cycle for each event.
// Returning an error triggers the retry logic.
//
// Example:
//
//	handler := func(ctx context.Context, event outbox.Event) error {
//	    return broker.Publish(ctx, event.Key, event.Payload)
//	}
type Handler func(ctx context.Context, event Event) error

// WithHandlerOptions applies configure to a matching handler configuration when
// a transport adapter resolves its options with ConfigureHandler. It can be
// mixed with ordinary Outbox options in the same slice.
func WithHandlerOptions[T any](configure func(*T)) Option {
	return func(o *options) {
		if target, ok := o.handlerConfig.(*T); ok {
			configure(target)
		}
	}
}

// ConfigureHandler applies adapter options to config at construction time.
func ConfigureHandler[T any](config *T, opts ...Option) {
	o := options{handlerConfig: config}
	for _, opt := range opts {
		opt(&o)
	}
}
