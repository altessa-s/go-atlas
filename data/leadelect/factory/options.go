// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
)

// --- Dependency methods ---

// UseLogger sets the logger for the builder and all created components.
func (b *LeaderBuilder) UseLogger(v *slog.Logger) *LeaderBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *LeaderBuilder) UseDefaultLogger() *LeaderBuilder {
	return b.UseLogger(slog.Default())
}

// UseNatsConn sets the NATS connection used for leader election.
func (b *LeaderBuilder) UseNatsConn(v *nats.Conn) *LeaderBuilder {
	b.natsConn = v
	return b
}

// --- Config methods ---

// WithKey sets the leader election key. Default is [appinfo.Name].
func (b *LeaderBuilder) WithKey(v string) *LeaderBuilder {
	if s := strings.TrimSpace(v); s != "" {
		b.key = s
	}
	return b
}

// WithNodeId sets the node identifier for leader election.
func (b *LeaderBuilder) WithNodeId(v string) *LeaderBuilder {
	if s := strings.TrimSpace(v); s != "" {
		b.nodeId = s
	}
	return b
}
