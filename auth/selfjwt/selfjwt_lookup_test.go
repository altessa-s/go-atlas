// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/selfjwt"
)

// blockingProvider signs with the wrapped provider but blocks VerificationKey
// until release is closed or the lookup context ends. It reports the lookup
// context's error on done.
type blockingProvider struct {
	*fakeProvider
	entered chan struct{}
	release chan struct{}
	done    chan error
}

func newBlockingProvider(t *testing.T) *blockingProvider {
	t.Helper()
	return &blockingProvider{
		fakeProvider: newProvider(t, selfjwt.AlgEdDSA),
		entered:      make(chan struct{}, 4),
		release:      make(chan struct{}),
		done:         make(chan error, 4),
	}
}

func (p *blockingProvider) VerificationKey(ctx context.Context, subject, kid string) (selfjwt.VerificationKey, error) {
	p.entered <- struct{}{}
	select {
	case <-p.release:
		p.done <- nil
		return p.fakeProvider.VerificationKey(ctx, subject, kid)
	case <-ctx.Done():
		p.done <- ctx.Err()
		return selfjwt.VerificationKey{}, ctx.Err()
	}
}

func mintFor(t *testing.T, p selfjwt.KeyProvider) string {
	t.Helper()
	res, err := selfjwt.NewMinter(p, clockOpt(baseTime)).Mint(t.Context(),
		selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)
	return res.Token
}

// TestVerifyHonorsCallerCancellationOnKeyLookup: a caller whose context is
// canceled while the key lookup is pending returns promptly with
// context.Canceled instead of waiting for the provider.
func TestVerifyHonorsCallerCancellationOnKeyLookup(t *testing.T) {
	t.Parallel()
	p := newBlockingProvider(t)
	v := selfjwt.NewVerifier(p, clockOpt(baseTime))
	token := mintFor(t, p)

	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := v.Verify(ctx, token)
		result <- err
	}()
	<-p.entered
	cancel()

	err := <-result // would block forever if the caller ignored its context
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, selfjwt.ErrTokenInvalid)

	close(p.release) // let the detached lookup finish
	require.NoError(t, <-p.done)
}

// TestVerifyKeyLookupIsBounded: the shared, detached lookup runs under its own
// deadline, so a provider that waits on its context terminates.
func TestVerifyKeyLookupIsBounded(t *testing.T) {
	t.Parallel()
	p := newBlockingProvider(t)
	v := selfjwt.NewVerifier(p, clockOpt(baseTime), selfjwt.WithKeyLookupTimeout(10*time.Millisecond))

	_, err := v.Verify(t.Context(), mintFor(t, p))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, <-p.done, context.DeadlineExceeded)
}

// TestVerifyCanceledWaiterDoesNotFailPeers: the waiter that started the shared
// lookup giving up must not cancel the lookup itself, which other waiters may
// share; its result is still delivered and cached.
func TestVerifyCanceledWaiterDoesNotFailPeers(t *testing.T) {
	t.Parallel()
	p := newBlockingProvider(t)
	v := selfjwt.NewVerifier(p, clockOpt(baseTime))
	token := mintFor(t, p)

	canceledCtx, cancel := context.WithCancel(t.Context())
	canceled := make(chan error, 1)
	go func() {
		_, err := v.Verify(canceledCtx, token)
		canceled <- err
	}()
	<-p.entered // the shared lookup is in flight
	cancel()
	require.ErrorIs(t, <-canceled, context.Canceled)

	// The lookup outlived its initiator: release it and it completes normally
	// (a canceled lookup context would report ctx.Err() on done instead).
	close(p.release)
	require.NoError(t, <-p.done)

	_, err := v.Verify(t.Context(), token)
	require.NoError(t, err)
	require.Len(t, p.entered, 0, "the completed lookup must have been cached")
}
