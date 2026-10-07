// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt_test

import (
	"context"
	"runtime"
	"strings"
	"sync/atomic"
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
	calls   atomic.Int32
	ctx     atomic.Pointer[context.Context] // lookup context of the latest call
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
	p.calls.Add(1)
	p.ctx.Store(&ctx)
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

// waitForKeyWaiters blocks until n goroutines are parked in the verifier's
// key-lookup wait, i.e. have joined the in-flight lookup.
func waitForKeyWaiters(t *testing.T, n int) {
	t.Helper()
	buf := make([]byte, 1<<20)
	for {
		stacks := string(buf[:runtime.Stack(buf, true)])
		if strings.Count(stacks, "selfjwt.(*Verifier).publicKey(") >= n {
			return
		}
		select {
		case <-t.Context().Done():
			t.Fatal("waiters never joined the key lookup")
		case <-time.After(time.Millisecond):
		}
	}
}

// TestVerifyCanceledWaiterDoesNotFailPeers: the waiter that started the shared
// lookup giving up neither cancels the lookup nor fails a peer that joined it.
//
// Not parallel: waitForKeyWaiters inspects every goroutine's stack, so other
// verifications in flight would be miscounted as waiters on this lookup.
func TestVerifyCanceledWaiterDoesNotFailPeers(t *testing.T) {
	p := newBlockingProvider(t)
	v := selfjwt.NewVerifier(p, clockOpt(baseTime), selfjwt.WithCacheTTL(0))
	token := mintFor(t, p)

	canceledCtx, cancel := context.WithCancel(t.Context())
	canceled := make(chan error, 1)
	go func() {
		_, err := v.Verify(canceledCtx, token)
		canceled <- err
	}()
	<-p.entered // the shared lookup is in flight

	live := make(chan error, 1)
	go func() {
		_, err := v.Verify(t.Context(), token)
		live <- err
	}()
	waitForKeyWaiters(t, 2) // the live peer is waiting on the same flight

	cancel()
	require.ErrorIs(t, <-canceled, context.Canceled)
	require.NoError(t, (*p.ctx.Load()).Err(), "the shared lookup context must outlive its initiator")

	close(p.release)
	require.NoError(t, <-live)
	require.NoError(t, <-p.done)
	require.Equal(t, int32(1), p.calls.Load(), "the peer must have shared the single lookup")
}
