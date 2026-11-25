// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

//go:generate go tool optgen generate --type=options

import (
	"crypto/tls"
	"log/slog"
	"net"
	"time"

	"github.com/altessa-s/go-atlas/transport/lifecycle"

	router "github.com/altessa-s/go-atlas/transport/http/server/router"
)

// Default timeout and size constants for HTTP server configuration.
const (
	// DefaultReadTimeout is the default timeout for reading the entire request.
	DefaultReadTimeout = 15 * time.Second

	// DefaultReadHeaderTimeout caps the time the server spends reading the
	// request headers — including the request line — independent of
	// [DefaultReadTimeout]. Required to defend against Slowloris-style
	// attacks where a malicious client trickles headers byte-by-byte to
	// pin a goroutine per connection within the full ReadTimeout window
	// (also resolves gosec G112).
	DefaultReadHeaderTimeout = 10 * time.Second

	// DefaultWriteTimeout is the default timeout for writing the response.
	DefaultWriteTimeout = 15 * time.Second

	// DefaultIdleTimeout is the default timeout for keep-alive connections.
	DefaultIdleTimeout = 60 * time.Second

	// DefaultMaxHeaderBytes is the default maximum size of request headers (1 MB).
	DefaultMaxHeaderBytes = 1 << 20
)

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
	lifecycleTimeouts lifecycle.Timeouts `optgen:"default=lifecycle.DefaultTimeouts()"`
	// WithListener serves a supplied listener and closes it on shutdown.
	listener net.Listener `optgen:"notnil"`

	router            router.Router `optgen:"notnil"`
	readTimeout       time.Duration `optgen:"default=DefaultReadTimeout"`
	readHeaderTimeout time.Duration `optgen:"default=DefaultReadHeaderTimeout"`
	writeTimeout      time.Duration `optgen:"default=DefaultWriteTimeout"`
	idleTimeout       time.Duration `optgen:"default=DefaultIdleTimeout"`
	maxHeaderBytes    int           `optgen:"default=DefaultMaxHeaderBytes"`
	writer            ResponseWriter
}
