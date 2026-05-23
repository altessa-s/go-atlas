// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"time"

	baseserver "github.com/altessa-s/go-atlas/transport/internal/server"
)

// Default timeout and size constants for HTTP server configuration.
const (
	// DefaultReadTimeout is the default timeout for reading the entire request.
	DefaultReadTimeout = 15 * time.Second

	// DefaultWriteTimeout is the default timeout for writing the response.
	DefaultWriteTimeout = 15 * time.Second

	// DefaultIdleTimeout is the default timeout for keep-alive connections.
	DefaultIdleTimeout = 60 * time.Second

	// DefaultMaxHeaderBytes is the default maximum size of request headers (1 MB).
	DefaultMaxHeaderBytes = 1 << 20
)

type options struct {
	baseOpts []baseserver.Option `opt:"BaseOptions"`

	router         Router        `optgen:"notnil"`
	readTimeout    time.Duration `optgen:"default=DefaultReadTimeout"`
	writeTimeout   time.Duration `optgen:"default=DefaultWriteTimeout"`
	idleTimeout    time.Duration `optgen:"default=DefaultIdleTimeout"`
	maxHeaderBytes int           `optgen:"default=DefaultMaxHeaderBytes"`
	writer         ResponseWriter
}
