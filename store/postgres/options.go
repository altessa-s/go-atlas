// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

//go:generate go tool optgen generate --type=options --no-default-modifiers
//go:generate go tool optgen generate --type=committerOptions --option-type=CommitterOption --output=committer_options_gen.go --no-default-modifiers

import (
	"crypto/tls"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/tracing"
)

// DefaultPool is the primary pool name in metrics and factory results.
const DefaultPool = "foreground"

// PoolingMode selects the lifetime of a PostgreSQL backend session.
type PoolingMode string

const (
	// PoolingDirect keeps one backend per connection.
	PoolingDirect PoolingMode = "direct"
	// PoolingTransaction leases a backend for each transaction.
	PoolingTransaction PoolingMode = "transaction"
)

// String returns the PostgreSQL pooling mode name.
func (m PoolingMode) String() string { return string(m) }

type options struct {
	// OnClose runs when database/sql closes the pool connector after a successful Open.
	onClose func()
	// WithTLSConfig requires TLS on every endpoint. An empty ServerName follows the endpoint host.
	tlsConfig *tls.Config
	// WithAllowInsecureSkipVerify permits an explicitly unverified TLS configuration.
	allowInsecureSkipVerify bool
	// Logger receives pool lifecycle messages.
	logger *slog.Logger `optgen:"default=slog.New(slog.DiscardHandler),notnil"`
	// Meter records database/sql pool statistics.
	meter metrics.Meter `optgen:"notnil"`
	// PoolName identifies the pool in metrics and logs.
	poolName string `optgen:"default=DefaultPool" optval:"trimspaces,nonempty"`
	// Tracer observes pgx queries on connections in the pool.
	tracer tracing.Tracer `optgen:"notnil"`
	// WithPooling selects direct connections or transaction pooling.
	pooling PoolingMode `optgen:"default=PoolingDirect"`
	// WithMaxOpenConnections limits the number of connections in this pool.
	maxOpenConnections int `optgen:"default=64"`
	// WithMaxIdleConnections limits idle connections retained in this pool.
	maxIdleConnections int `optgen:"default=64"`
	// WithConnectionMaxLifetime limits the age of a connection.
	connectionMaxLifetime time.Duration `optgen:"default=30*time.Minute"`
	// WithConnectionMaxIdleTime limits how long a connection can stay idle.
	connectionMaxIdleTime time.Duration `optgen:"default=5*time.Minute"`
	// WithConnectTimeout bounds connection establishment.
	connectTimeout time.Duration `optgen:"default=5*time.Second"`
	// WithStatementTimeout bounds each statement in a transaction or direct session.
	statementTimeout time.Duration `optgen:"default=10*time.Second"`
	// WithLockTimeout bounds PostgreSQL lock acquisition.
	lockTimeout time.Duration `optgen:"default=3*time.Second"`
	// WithIdleTransactionTimeout bounds idle time in an open transaction.
	idleTransactionTimeout time.Duration `optgen:"default=30*time.Second"`
}

type committerOptions struct {
	// WithTxRetries sets the maximum additional attempts after serialization failures or deadlocks.
	txRetries int
	// WithTxRetryDelay sets the initial exponential backoff between transaction attempts.
	txRetryDelay time.Duration `optgen:"default=10*time.Millisecond"`
	// WithTxTimeout caps the total time for the transaction and all retries. Zero uses the caller's deadline.
	txTimeout time.Duration
}
