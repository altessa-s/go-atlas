// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"

	"github.com/nats-io/nats.go"

	"github.com/altessa-s/go-atlas/coord/leadelect"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// NewLeader creates a [leadelect.Leader] for key and nodeID over a NATS
// provider built from conn with default provider options. opts configure the
// Leader; to tune the provider, build it with [New] and pass it to
// [leadelect.New].
//
// Example:
//
//	le, err := nats.NewLeader(ctx, conn, "my-service", "node-1")
func NewLeader(ctx context.Context, conn *nats.Conn, key, nodeID string, opts ...leadelect.Option) (*leadelect.Leader, error) {
	prov, err := New(ctx, conn)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create nats leader elector")
	}
	return leadelect.New(prov, key, nodeID, opts...), nil
}
