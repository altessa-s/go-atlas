// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	idempotencydata "github.com/altessa-s/go-atlas/reliability/idempotency"
	storages "github.com/altessa-s/go-atlas/reliability/idempotency/storages"
)

// ctxKeeper fails Complete and Release under an ended context, as a storage
// reached over the network does.
type ctxKeeper struct{ *idempotencydata.Keeper }

func (k ctxKeeper) Complete(ctx context.Context, key string, data any, state *storages.State) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return k.Keeper.Complete(ctx, key, data, state)
}

func (k ctxKeeper) Release(ctx context.Context, key string, state *storages.State) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return k.Keeper.Release(ctx, key, state)
}

// A client that disconnects while the handler runs cancels the call's
// context; the key is still completed or released.
func TestCanceledCallStillFinalizesKey(t *testing.T) {
	t.Parallel()

	errHandler := errors.New("handler failed")
	tests := []struct {
		name       string
		handlerErr error
		wantRepeat codes.Code
		wantCalls  int
	}{
		{name: "success completes the key", wantRepeat: codes.FailedPrecondition, wantCalls: 1},
		{name: "failure releases the key", handlerErr: errHandler, wantRepeat: codes.OK, wantCalls: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ic := idempotency.ServerInterceptor(ctxKeeper{newReplayKeeper()}).ServerUnaryInterceptor()
			ctx, cancel := context.WithCancel(t.Context())
			calls := 0
			_, err := unary(t, ic, ctx, getReq("order"), func(context.Context, any) (any, error) {
				calls++
				cancel()
				return getReq("order"), tc.handlerErr
			})
			require.ErrorIs(t, err, tc.handlerErr)

			_, err = unary(t, ic, t.Context(), getReq("order"), func(context.Context, any) (any, error) {
				calls++
				return getReq("order"), nil
			})
			require.Equal(t, tc.wantRepeat, status.Code(err))
			require.Equal(t, tc.wantCalls, calls)
		})
	}
}

// A panicking handler never reaches PostCall; its lock is released while the
// panic unwinds, so a retry runs at once.
func TestPanickingHandlerReleasesKey(t *testing.T) {
	t.Parallel()

	ic := idempotency.ServerInterceptor(newReplayKeeper()).ServerUnaryInterceptor()
	require.PanicsWithValue(t, "boom", func() {
		_, _ = unary(t, ic, t.Context(), getReq("order"), func(context.Context, any) (any, error) { //nolint:errcheck // panics
			panic("boom")
		})
	})

	h := &counter{}
	_, err := unary(t, ic, t.Context(), getReq("order"), h.handle)
	require.NoError(t, err)
	require.EqualValues(t, 1, h.calls.Load())
}
