// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

//go:generate go tool optgen generate --type=options

import (
	"crypto/tls"
	"log/slog"
	"net"
	"time"

	"github.com/altessa-s/go-atlas/internal/transport/timeouts"
	"github.com/altessa-s/go-atlas/transport/lifecycle"

	"google.golang.org/grpc"
)

// DefaultStartupVerification is how long [Server.Start] waits for the serving
// goroutine to fail before reporting a successful start.
const DefaultStartupVerification = timeouts.DefaultStartupVerification

type options struct {
	// WithAddress sets the TCP listen address. The default is ":0".
	address string `optval:"lower"`
	// WithName sets the server name used in lifecycle logs.
	name string `optval:"lower"`
	// WithTLSConfig configures TLS for supplied and automatically created listeners.
	tlsConfig *tls.Config
	// WithLogger sets the logger for server lifecycle events.
	logger *slog.Logger
	// WithLifecycleTimeouts sets the startup verification and shutdown durations.
	lifecycleTimeouts lifecycle.Timeouts `optgen:"default=lifecycle.DefaultTimeouts(),enables=lifecycleTimeoutsSet"`
	// Set by WithLifecycleTimeouts, which takes precedence over WithStartupVerification.
	lifecycleTimeoutsSet bool `opt:"-"`

	// WithGrpcOptions appends options to pass to the underlying gRPC server in order.
	grpcOpts   []grpc.ServerOption `opt:"GrpcOptions" optgen:"append" optval:"nodup"`
	reflection bool                `opt:"Reflection" optgen:"default=false"`

	// listener, when set, is served instead of a TCP listener on the
	// configured address — e.g. a bufconn listener in tests. The server
	// closes it on shutdown.
	listener net.Listener `optgen:"notnil"`

	// startupVerification is how long Start waits for the serving goroutine
	// to report an early error before declaring the start successful. Zero
	// or negative skips the wait (an early serve error is then only logged).
	// WithLifecycleTimeouts takes precedence.
	startupVerification time.Duration `optgen:"default=DefaultStartupVerification" optval:"nonpositive"`
}
