// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/altessa-s/go-atlas/observability/metrics"

	lockconfig "github.com/altessa-s/go-atlas/config/lock"
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

// UseProvider registers the provider function of one provider type
// (lockconfig.LeaderElectorProviderNATS). A later registration of the same
// type replaces the earlier one.
func (b *LeaderBuilder) UseProvider(typ lockconfig.LeaderElectorProvider, build ProviderFunc) *LeaderBuilder {
	if build == nil {
		b.errs = append(b.errs, fmt.Errorf("UseProvider %q: nil function", typ))
		return b
	}
	if b.providers == nil {
		b.providers = map[lockconfig.LeaderElectorProvider]ProviderFunc{}
	}
	b.providers[typ] = build
	return b
}

// UseMeter sets the [metrics.Meter] for recording leader election metrics.
func (b *LeaderBuilder) UseMeter(v metrics.Meter) *LeaderBuilder {
	b.collector = v
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
