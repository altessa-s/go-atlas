// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package callstatus shares the pre-conversion error with server RPC observers.
package callstatus

import (
	"context"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc/status"
)

type causeKey struct{}

// Observe opens a slot for the original error on this server call. The
// interceptor that converts the error writes it as the handler returns,
// before the synchronous completion hooks read it.
func Observe(ctx context.Context) context.Context {
	if _, ok := ctx.Value(causeKey{}).(*error); ok {
		return ctx
	}
	return context.WithValue(ctx, causeKey{}, new(error))
}

// Save records err before the service converter or finalizer can replace it.
func Save(ctx context.Context, err error) {
	if slot, ok := ctx.Value(causeKey{}).(*error); ok {
		if ie, ok := err.(*interceptors.Error); ok {
			err = ie.Unwrap()
		}
		*slot = err
	}
}

// Cause returns the error recorded on this call, if any.
func Cause(ctx context.Context) error {
	if slot, ok := ctx.Value(causeKey{}).(*error); ok {
		return *slot
	}
	return nil
}

// Status resolves the status just as grpc-go does when sending a server error.
func Status(err error) *status.Status {
	if st, ok := status.FromError(err); ok {
		return st
	}
	return status.FromContextError(err)
}
