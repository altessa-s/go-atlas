// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
)

// ClientFactory creates a new gRPC client connection for the given target address.
// The context carries the connection timeout configured via [WithConnectTimeout].
// When no ClientFactory is set, the pool dials with [insecure.NewCredentials].
type ClientFactory func(ctx context.Context, target string) (*grpc.ClientConn, error)

// Default values for connection pool options.
const (
	// DefaultPoolSize is the default number of connections in the pool.
	DefaultPoolSize = 10

	// DefaultMaxIdleTime is the default maximum idle time before a connection is closed.
	DefaultMaxIdleTime = 30 * time.Minute

	// DefaultCleanupInterval is the default interval for cleaning up idle connections.
	DefaultCleanupInterval = 5 * time.Minute

	// DefaultConnectTimeout is the default timeout for establishing a connection.
	DefaultConnectTimeout = 10 * time.Second
)

// options holds configuration for the connection pool.
type options struct {
	// size sets the number of connections in the pool.
	size int `optgen:"default=DefaultPoolSize,min=1"`
	// maxIdleTime sets the maximum idle time before a connection is closed.
	maxIdleTime time.Duration `optgen:"default=DefaultMaxIdleTime,min=1s"`
	// cleanupInterval sets the interval for cleaning up idle connections.
	cleanupInterval time.Duration `optgen:"default=DefaultCleanupInterval,min=1s"`
	// connectTimeout sets the timeout for establishing a connection.
	connectTimeout time.Duration `optgen:"default=DefaultConnectTimeout,min=1s"`
	// logger sets the logger for the pool.
	logger *slog.Logger
	// clientFactory sets a custom client factory function for creating gRPC connections.
	clientFactory ClientFactory
}
