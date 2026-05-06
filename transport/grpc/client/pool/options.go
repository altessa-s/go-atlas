// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import (
	"context"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"

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
	// collector sets the metrics collector for the pool.
	collector metrics.Collector `optgen:"notnil"`
	// metricsSubsystem sets the Prometheus subsystem name for emitted metrics.
	metricsSubsystem string `optgen:"default=DefaultMetricsSubsystem"`
	// healthCoordinator opts the pool into [observability/health] integration.
	// When non-nil the pool registers an aggregate [health.Checker] that
	// reflects the worst per-target status across all tracked connections.
	// Leave nil to disable the integration entirely.
	healthCoordinator *health.Coordinator
	// healthServiceName is the service name used when registering the
	// aggregate checker. Defaults to [DefaultPoolHealthServiceName].
	healthServiceName string `optgen:"default=DefaultPoolHealthServiceName"`
	// healthPerTarget toggles per-target service registration in addition to
	// the aggregate one. Per-target services are registered lazily on the
	// first conn for a target and removed when its last conn is closed.
	// Set via [WithHealthPerTarget].
	healthPerTarget bool `optgen:"notnil"`
	// healthStateMapper maps [connectivity.State] to [health.ServingStatus].
	// Set via [WithHealthStateMapper]. A nil value is rejected by the
	// option; the helper falls back to [DefaultStateMapper] when the field
	// is unset.
	healthStateMapper StateMapper `optgen:"notnil,default=DefaultStateMapper"`
}
