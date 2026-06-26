// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package eventbus

import (
	"context"
	"errors"
	"fmt"
)

// ErrNotInTransaction is returned by PublishTx when the bus reports no active
// transaction for the context. It is matchable with errors.Is.
var ErrNotInTransaction = errors.New("eventbus: publish requires an active transaction")

// TxProbe reports whether ctx carries an active transaction for some backend.
// The bus stays storage-agnostic: each backend (MongoDB, etc.) supplies its own
// probe at construction via WithTxProbe; compose several with AnyInTx.
type TxProbe func(ctx context.Context) bool

// AnyInTx composes probes into one that reports an active transaction if ANY of
// the non-nil probes does. Nil probes are skipped; with no probes the result is
// always false. Use it to support multiple backends:
//
//	eventbus.New(eventbus.WithTxProbe(eventbus.AnyInTx(mongoProbe, pgProbe)))
func AnyInTx(probes ...TxProbe) TxProbe {
	nonNil := make([]TxProbe, 0, len(probes))
	for _, p := range probes {
		if p != nil {
			nonNil = append(nonNil, p)
		}
	}
	return func(ctx context.Context) bool {
		for _, p := range nonNil {
			if p(ctx) {
				return true
			}
		}
		return false
	}
}

// WithTxProbe sets the bus's transaction probe. It is the only place a concrete
// backend is wired in; the bus core never imports a database driver.
func WithTxProbe(probe TxProbe) Option {
	return func(eb *InProcessBus) { eb.inTx = probe }
}

// InTransaction reports whether ctx carries an active transaction, per the
// probe configured with WithTxProbe. It is false when no probe is configured.
func (eb *InProcessBus) InTransaction(ctx context.Context) bool {
	return eb.inTx != nil && eb.inTx(ctx)
}

// PublishTx publishes a typed event only when the bus reports an active
// transaction for ctx; otherwise it returns ErrNotInTransaction without
// dispatching. The probe is an attribute of the bus instance (WithTxProbe), so
// callers do not pass it per publish. Use PublishTx for transactional commands
// (a subscriber must run in the same transaction); use Publish for fire-and-forget
// notifications.
func PublishTx[E any](ctx context.Context, b Bus, event E) error {
	if !b.InTransaction(ctx) {
		return fmt.Errorf("%w: %T", ErrNotInTransaction, event)
	}
	return b.Publish(ctx, event)
}
