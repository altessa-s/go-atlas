// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mirror

import (
	"log/slog"
	"time"
)

//go:generate go tool optgen generate

// DefaultMaxStaleness is the oldest snapshot a [Cache] trusts when
// [WithMaxStaleness] is not set. Pass zero to [WithMaxStaleness] to disable the
// bound explicitly.
const DefaultMaxStaleness = 5 * time.Minute

// options carries the optional [Cache] tunables.
type options struct {
	// metrics records refresh outcomes and snapshot size. Nil disables metrics —
	// every recording becomes a no-op. Set via the generated WithMetrics.
	metrics *Metrics `optgen:"notnil"`
	// logger reports a change between serving and refusing to answer. Nil keeps
	// the default, which discards. Set via the generated WithLogger.
	logger *slog.Logger `optgen:"notnil"`
	// clock is the time source for snapshot age. Set via the generated
	// WithClock; a nil clock is ignored.
	clock func() time.Time `optgen:"default=time.Now,notnil"`
	// maxStaleness is the oldest snapshot [Cache.IsRevoked] still trusts; an
	// older one answers [github.com/altessa-s/go-atlas/auth/jwt.ErrRevocationUnavailable].
	// Defaults to [DefaultMaxStaleness]; zero disables the bound explicitly. Set
	// via the generated WithMaxStaleness.
	maxStaleness time.Duration `optcheck:"min=0" optgen:"default=DefaultMaxStaleness,onfail=skip"`
	// serveUnprimed lets an unprimed cache answer "not revoked" instead of
	// refusing. It is written by the hand-written [WithInsecureServeUnprimed],
	// whose name states the risk and which takes no argument.
	serveUnprimed bool `optgen:"manual"`
}

// WithInsecureServeUnprimed lets a cache that has never refreshed successfully
// answer every lookup "not revoked" instead of refusing with
// [github.com/altessa-s/go-atlas/auth/jwt.ErrRevocationUnavailable]. Until the
// first successful [Cache.Prime] or [Cache.Refresh] no revocation is enforced,
// so use it only where an outage of the source at startup must not block
// authentication.
func WithInsecureServeUnprimed() Option {
	return func(o *options) {
		o.serveUnprimed = true
	}
}
