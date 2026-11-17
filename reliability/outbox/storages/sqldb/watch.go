// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/reliability/outbox"

	coreretry "github.com/altessa-s/go-atlas-core/retry"
)

// Listener subscribes to PostgreSQL notifications. database/sql has no
// LISTEN API, so the store takes one from the caller — typically a thin
// adapter over a dedicated driver connection, such as pgx's
// Conn.WaitForNotification; see the package README for one.
//
// Listen must return only once the subscription is active, so that every
// notification committed after it returns is delivered. The returned channel
// receives a value per notification on channel — an adapter may coalesce a
// burst into one value, and should never block on a full channel — and is
// closed when the subscription ends: ctx done, or the connection lost. An
// error means no subscription was established. Listen must return a non-nil
// channel on success; a nil channel is treated as a closed subscription and retried.
type Listener interface {
	Listen(ctx context.Context, channel string) (<-chan struct{}, error)
}

// Sentinel errors returned by [New] for the notification options.
var (
	// ErrNotifyUnsupported is returned when [WithNotify], [WithNotifyChannel]
	// or [WithListener] is used with a dialect other than [DialectPostgres]:
	// MySQL and MariaDB have no LISTEN/NOTIFY.
	ErrNotifyUnsupported = errors.New("sqldb: notifications require the postgres dialect")
	// ErrInvalidNotifyChannel is returned when the notification channel — set
	// with [WithNotifyChannel] or derived from the table name — is not a plain
	// identifier of at most 63 characters.
	ErrInvalidNotifyChannel = errors.New("sqldb: invalid notification channel")
)

// errListenerClosed reports a subscription that ended while the watcher was
// still running.
var errListenerClosed = errors.New("sqldb: notification subscription closed")

// Watch reconnect timings. Deliberately not options: the window they govern is
// bounded by the dispatch schedule either way, since the poll cycle keeps
// draining the outbox while no subscription is active.
const (
	// watchRetryBaseDelay is the pause before the first resubscription.
	watchRetryBaseDelay = time.Second
	// watchRetryMaxDelay caps the pause between resubscriptions.
	watchRetryMaxDelay = 30 * time.Second
	// watchRetryFactor is the exponential multiplier applied per attempt.
	watchRetryFactor = 1.5
	// watchRetryJitter spreads the resubscriptions of the instances that lost
	// the same server.
	watchRetryJitter = 0.2
	// watchLogEvery spaces the warnings of a run of consecutive failures: the
	// first one is logged, then every watchLogEvery-th.
	watchLogEvery = 10
)

// notifyChannel returns the validated notification channel: explicit, or
// derived from the table name by joining a schema qualifier with an underscore
// and lowering the case, so an unquoted LISTEN in psql names it too.
func notifyChannel(explicit, table string) (string, error) {
	ch := explicit
	if ch == "" {
		ch = strings.ToLower(strings.ReplaceAll(table, ".", "_"))
	}
	if strings.Contains(ch, ".") {
		return "", fmt.Errorf("%w: %q", ErrInvalidNotifyChannel, ch)
	}
	if _, err := sqldialect.Postgres.Table(ch); err != nil {
		return "", fmt.Errorf("%w: %q: set one with WithNotifyChannel", ErrInvalidNotifyChannel, ch)
	}
	return ch, nil
}

// NotifyChannel returns the PostgreSQL notification channel the store signals
// on save and [Store.Watch] listens to; empty on MySQL.
func (s *Store) NotifyChannel() string {
	return s.channel
}

// SupportsNotifications reports whether [Store.Watch] can deliver
// notifications: a [Listener] was set with [WithListener]. It answers without
// I/O; a watcher whose subscription fails keeps retrying instead.
func (s *Store) SupportsNotifications() bool {
	return s.listener != nil
}

// Watch implements [outbox.Watcher] with PostgreSQL LISTEN/NOTIFY through the
// [Listener] set with [WithListener], so a saved event wakes the dispatcher
// instead of waiting for its next poll tick. Savers signal the channel when
// [WithNotify] is set — in this process or any other writing the same table.
// It blocks until ctx is done and returns nil on a clean shutdown.
//
// Returns [outbox.ErrWatchUnsupported] when no Listener is set.
//
// The signal is sent inside the saving transaction, so it is delivered when
// that transaction commits and never for one that rolls back. A subscription
// that fails or ends is reopened after an exponential backoff, followed by one
// synthetic wake-up, since notifications sent while nobody listened are lost.
func (s *Store) Watch(ctx context.Context, notify func()) error {
	if s.listener == nil {
		return outbox.ErrWatchUnsupported
	}

	backoff := coreretry.Exponential(
		watchRetryBaseDelay,
		coreretry.WithExponentialMaxDelay(watchRetryMaxDelay),
		coreretry.WithExponentialFactor(watchRetryFactor),
		coreretry.WithExponentialJitter(watchRetryJitter),
	)

	var (
		attempt int
		gap     bool
	)
	for {
		signals, err := s.listener.Listen(ctx, s.channel)
		if ctx.Err() != nil {
			return nil
		}
		if err == nil && signals == nil {
			err = errListenerClosed
		}
		if err == nil {
			started := time.Now()
			if gap {
				notify() // Events committed while nobody listened.
			}
			if received := pump(ctx, signals, notify); received && time.Since(started) >= watchRetryMaxDelay {
				// Only a stable subscription that delivered a signal resets the
				// backoff: a silent one may have been half-open all along. On a
				// quiet outbox the backoff then stays high, at most watchRetryMaxDelay.
				attempt = 0
			}
			if ctx.Err() != nil {
				return nil
			}
			err = errListenerClosed
		}

		gap = true
		delay := backoff(attempt, err)
		attempt++
		if attempt == 1 || attempt%watchLogEvery == 0 {
			s.logger.WarnContext(ctx, "outbox notification subscription failed, resubscribing",
				slog.Int("failures", attempt), slog.Duration("delay", delay), slog.Any("error", err))
		}
		if !sleep(ctx, delay) {
			return nil
		}
	}
}

// pump calls notify per signal until signals closes or ctx ends, reporting
// whether any signal arrived.
func pump(ctx context.Context, signals <-chan struct{}, notify func()) (received bool) {
	for {
		select {
		case <-ctx.Done():
			return received
		case _, ok := <-signals:
			if !ok {
				return received
			}
			received = true
			notify()
		}
	}
}

// sleep waits for d, reporting false when ctx ended first.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
