// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"context"

	"github.com/altessa-s/go-atlas/transport/grpc/client/pool"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// ConnPool binds a dial policy to a target, accepts returned connections,
// and reports the target's connectivity to health subscribers.
type ConnPool interface {
	Bind(string, pool.ClientFactory) (pool.Binding, error)
	ReturnConnection(*grpc.ClientConn)
	StateForTarget(string) connectivity.State
	SubscribeTarget(string, func(connectivity.State)) (func(), error)
}

type connBackend interface {
	GetConnection(context.Context) (*grpc.ClientConn, error)
	ReturnConnection(*grpc.ClientConn)
	Close() error
	GetState() connectivity.State
	attachHealth(context.Context, *clientHealth) error
}

type singleBackend struct{ *grpc.ClientConn }

func (b *singleBackend) GetConnection(context.Context) (*grpc.ClientConn, error) {
	return b.ClientConn, nil
}
func (*singleBackend) ReturnConnection(*grpc.ClientConn) {}

type pooledBackend struct {
	pool    ConnPool
	binding pool.Binding
	target  string
}

func (b *pooledBackend) GetConnection(ctx context.Context) (*grpc.ClientConn, error) {
	return b.binding.GetConnection(ctx)
}
func (b *pooledBackend) ReturnConnection(conn *grpc.ClientConn) { b.pool.ReturnConnection(conn) }
func (b *pooledBackend) GetState() connectivity.State           { return b.pool.StateForTarget(b.target) }

// The pool retains ownership of its connections.
func (*pooledBackend) Close() error { return nil }
