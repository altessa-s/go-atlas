// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/idempotency"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages/memory"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	schedulerv1 "github.com/altessa-s/go-atlas/proto/gen/scheduler/v1"
	sharedmetadata "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
	grpcmetadata "google.golang.org/grpc/metadata"
)

// A keyed client-streaming or bidi call receives many messages; only the
// first one takes the lock, and the call state is safe to touch from the
// goroutine that receives later messages.
func TestStreamingCallChecksOncePerCall(t *testing.T) {
	t.Parallel()

	keeper := idempotency.New(memory.New(), idempotency.WithMaxLockDuration(time.Minute))
	opts := newOptions(WithRequestFingerprint(true))
	opts.statusCreator = defaultStatusCreator
	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptor(interceptorName, nil),
		i:               keeper,
		opts:            opts,
	}
	ctx := grpcmetadata.NewIncomingContext(t.Context(),
		grpcmetadata.Pairs(DefaultIdempotencyKeyHeader, "0b8a4f7e-6c1d-4e2a-9b3c-5d6e7f8a9b0c"))
	ri := &requestInterceptor{
		interceptor: ic,
		meta:        &sharedmetadata.CallMetadata{FullyMethodName: schedulerv1.SchedulerService_Get_FullMethodName},
	}

	require.NoError(t, ri.PostMsgReceive(ctx, &schedulerv1.TaskGetRequest{TaskId: "a"}, nil))

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Go(func() {
			errs[i] = ri.PostMsgReceive(ctx, &schedulerv1.TaskGetRequest{TaskId: "b"}, nil)
		})
	}
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])

	require.NoError(t, ri.PostCall(ctx, nil, nil))
}

func newStreamCall(t *testing.T, keeper *idempotency.Keeper) *requestInterceptor {
	t.Helper()
	opts := newOptions()
	opts.statusCreator = defaultStatusCreator
	return &requestInterceptor{
		interceptor: &interceptor{BaseInterceptor: interceptors.NewBaseInterceptor(interceptorName, nil), i: keeper, opts: opts},
		meta:        &sharedmetadata.CallMetadata{FullyMethodName: schedulerv1.SchedulerService_Get_FullMethodName},
	}
}

func keyedContext(t *testing.T) context.Context {
	t.Helper()
	return grpcmetadata.NewIncomingContext(t.Context(),
		grpcmetadata.Pairs(DefaultIdempotencyKeyHeader, "0b8a4f7e-6c1d-4e2a-9b3c-5d6e7f8a9b0c"))
}

// A first receive that runs after the call finished takes no lock: nothing
// would be left to complete or release it.
func TestLateFirstReceiveTakesNoLock(t *testing.T) {
	t.Parallel()

	keeper := idempotency.New(memory.New(), idempotency.WithMaxLockDuration(time.Minute))
	ctx := keyedContext(t)
	late := newStreamCall(t, keeper)
	gate := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		<-gate
		done <- late.PostMsgReceive(ctx, &schedulerv1.TaskGetRequest{TaskId: "a"}, nil)
	}()

	require.NoError(t, late.PostCall(ctx, nil, nil))
	close(gate)
	require.NoError(t, <-done)

	// The key is free: another call acquires it instead of being refused as in progress.
	require.NoError(t, newStreamCall(t, keeper).PostMsgReceive(ctx, &schedulerv1.TaskGetRequest{TaskId: "a"}, nil))
}

// A receive and the end of the call overlap: with -race there is no data
// race, and the key never stays locked by a finished call.
func TestReceiveOverlapsPostCall(t *testing.T) {
	t.Parallel()

	for range 50 {
		keeper := idempotency.New(memory.New(), idempotency.WithMaxLockDuration(time.Minute))
		ctx := keyedContext(t)
		ri := newStreamCall(t, keeper)
		var wg sync.WaitGroup
		wg.Go(func() { _ = ri.PostMsgReceive(ctx, &schedulerv1.TaskGetRequest{TaskId: "a"}, nil) })
		wg.Go(func() { _ = ri.PostCall(ctx, nil, errors.New("handler failed")) })
		wg.Wait()

		// Failed call: the key is released or was never taken.
		require.NoError(t, newStreamCall(t, keeper).PostMsgReceive(ctx, &schedulerv1.TaskGetRequest{TaskId: "a"}, nil))
	}
}
