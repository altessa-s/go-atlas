// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"

	"github.com/nats-io/nats.go"

	"github.com/altessa-s/go-atlas/coord/dlock"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// NewDLock creates a [dlock.DLock] over a NATS JetStream provider that keeps
// its locks in bucket. opts configure the DLock; to pass other provider
// options, build the provider with [New] and pass it to [dlock.New].
//
// Example:
//
//	dl, err := nats.NewDLock(ctx, conn, "locks")
func NewDLock(ctx context.Context, conn *nats.Conn, bucket string, opts ...dlock.Option) (*dlock.DLock, error) {
	prov, err := New(ctx, conn, WithBucket(bucket))
	if err != nil {
		return nil, coreerrs.Provider("nats distributed lock", err)
	}
	return dlock.New(prov, opts...), nil
}
