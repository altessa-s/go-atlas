// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spoolbudget

import (
	"context"
	"io"

	"github.com/altessa-s/go-atlas/core/io/spool"

	"golang.org/x/sync/semaphore"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Budget bounds the total bytes concurrently held on disk by spools across
// every call site. A nil *Budget (or one built with a non-positive limit) is
// disabled and imposes no bound, so callers can invoke its methods without a
// nil check. Budget is safe for concurrent use.
type Budget struct {
	sem   *semaphore.Weighted
	limit int64
}

// New returns a Budget limiting the aggregate held spool bytes to limitBytes.
// A non-positive limit returns nil — a disabled budget that callers may use
// directly.
func New(limitBytes int64) *Budget {
	if limitBytes <= 0 {
		return nil
	}
	return &Budget{sem: semaphore.NewWeighted(limitBytes), limit: limitBytes}
}

// Limit reports the configured byte ceiling, or 0 when the budget is disabled.
func (b *Budget) Limit() int64 {
	if b == nil {
		return 0
	}
	return b.limit
}

// BudgetedSpool is a [spool.Spool] whose materialized size is charged against a
// [Budget] until Close. When the budget is disabled it behaves exactly like the
// underlying spool.
type BudgetedSpool struct {
	*spool.Spool
	release func()
}

// Close releases the budgeted bytes (once) and then closes the underlying spool.
func (s *BudgetedSpool) Close() error {
	if s.release != nil {
		s.release()
		s.release = nil
	}
	return s.Spool.Close()
}

// Spool reserves maxBytes against the shared budget BEFORE materializing r via
// [spool.New], then releases the unused remainder once the real size is known,
// holding only the actual size until Close. Reserving the per-spool cap up
// front — rather than the size after writing — is what makes the budget a true
// peak bound: a spool cannot begin writing until its whole cap fits, so the sum
// of on-disk bytes across all in-flight spools never exceeds the limit. maxBytes
// caps the spool itself too (applied as [spool.WithMaxBytes]); an oversize r
// fails with [spool.ErrTooLarge]. ctx governs the wait for the reservation.
//
// A disabled budget (nil receiver / non-positive limit) or a non-positive
// maxBytes (unbounded per-spool) skips the reservation and just materializes r.
// When the budget is enabled maxBytes must be positive and not exceed the limit,
// so the reservation can always be satisfied; a maxBytes larger than the limit
// could only end when ctx is canceled.
func (b *Budget) Spool(ctx context.Context, r io.Reader, maxBytes int64, opts ...spool.Option) (*BudgetedSpool, error) {
	reserve := b != nil && b.sem != nil && maxBytes > 0
	if reserve {
		if aerr := b.sem.Acquire(ctx, maxBytes); aerr != nil {
			return nil, coreerrs.WrapOperation(aerr, "reserve spool disk budget")
		}
	}
	if maxBytes > 0 {
		opts = append(opts, spool.WithMaxBytes(maxBytes))
	}
	sp, err := spool.New(r, opts...)
	if err != nil {
		if reserve {
			b.sem.Release(maxBytes)
		}
		return nil, err
	}
	if !reserve {
		return &BudgetedSpool{Spool: sp}, nil
	}
	// The spool is on disk and is at most maxBytes; release the over-reservation
	// now and hold only the actual size until Close.
	held := sp.Size()
	if excess := maxBytes - held; excess > 0 {
		b.sem.Release(excess)
	}
	return &BudgetedSpool{Spool: sp, release: func() { b.sem.Release(held) }}, nil
}
