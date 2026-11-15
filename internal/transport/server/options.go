// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

import (
	"crypto/tls"
	"log/slog"
	"net"

	"github.com/altessa-s/go-atlas/internal/transport/timeouts"
)

// Constants for default server options.
const (
	// DefaultAddress is the default server address.
	DefaultAddress = ":0"
	// DefaultName is the default server name.
	DefaultName = ""
)

//go:generate go tool optgen generate --type=options

// options holds [BaseServer] configuration populated by functional
// [Option] values via optgen.
type options struct {
	tlsConfig *tls.Config
	listener  net.Listener `optgen:"notnil"`
	logger    *slog.Logger
	address   string             `optgen:"default=DefaultAddress" optval:"lower"`
	name      string             `optgen:"default=DefaultName" optval:"lower"`
	timeouts  timeouts.Lifecycle `optgen:"default=timeouts.Default()"`
}
