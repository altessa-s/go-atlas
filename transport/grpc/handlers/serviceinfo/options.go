// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package serviceinfo

import (
	"context"

	_ "github.com/altessa-s/go-atlas/core/runtime/appinfo"
)

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

// LeaderProvider is the consumer-side contract for leader-election state.
// The method names match data/leadelect.Leader so that *leadelect.Leader
// satisfies this interface as-is, with no adapter required. Custom
// implementations (Raft, etcd, external coordinator) can be supplied
// through [WithLeaderProvider].
type LeaderProvider interface {
	// IsLeader reports whether this instance currently holds leadership.
	IsLeader() bool
	// LeaderId returns the identifier of the current leader. The spelling
	// LeaderId (not LeaderID) is intentional: it mirrors data/leadelect.Leader
	// so that type satisfies the interface natively.
	LeaderId(ctx context.Context) (string, error)
}

// LeaderFunc is the function shape accepted by [WithLeader] for callers
// that don't have a [LeaderProvider]-shaped object. It is invoked once
// per Get request — keep it cheap. Returning a non-nil error from the
// internal LeaderId call is impossible via this adapter, so the error
// path is never exercised; the handler treats an empty leaderID as
// "leader unknown" and falls back to the configured serviceID.
type LeaderFunc func(ctx context.Context) (isLeader bool, leaderID string)

// WithLeader is a convenience option for callers that want to plug
// leader state in via a single function instead of a [LeaderProvider]
// implementation. Mutually exclusive with [WithLeaderProvider] — the
// last applied option wins. A nil fn is ignored.
func WithLeader(fn LeaderFunc) Option {
	if fn == nil {
		return func(*options) {}
	}
	return func(o *options) {
		o.leaderProvider = leaderFnAdapter{fn: fn}
	}
}

// leaderFnAdapter satisfies [LeaderProvider] by invoking the wrapped
// function on every call. Note that handler.Get calls IsLeader and then
// LeaderId, which means fn is invoked twice per request. The default
// LeaderProvider call site is not on a hot path, so this is acceptable.
type leaderFnAdapter struct {
	fn LeaderFunc
}

func (a leaderFnAdapter) IsLeader() bool {
	isLeader, _ := a.fn(context.Background())
	return isLeader
}

func (a leaderFnAdapter) LeaderId(ctx context.Context) (string, error) {
	_, id := a.fn(ctx)
	return id, nil
}

// options holds tunables for [Handler]. All fields are optional; safe
// defaults are derived from [appinfo].
type options struct {
	// serviceName is reported in ServiceInfo.service_name.
	// Defaults to appinfo.Name (set via -ldflags at build time).
	serviceName string `optgen:"default=appinfo.Name"`

	// serviceDescription is reported in ServiceInfo.service_description.
	// Empty value is omitted from the response.
	serviceDescription string

	// serviceID is reported in ServiceInfo.service_id and used as the
	// leader_id when the local instance is elected leader. Empty value
	// is omitted from both fields.
	serviceID string

	// leaderProvider supplies leader state. Nil disables the
	// is_leader / leader_id fields entirely.
	leaderProvider LeaderProvider

	// extraMetadata is merged on top of the runtime/build metadata
	// populated by [appinfo]. Conflicting keys from the caller win.
	// To surface the version of a proto module, pass it here under a
	// "proto_version" key — the handler intentionally does not look up
	// dep versions on the caller's behalf.
	extraMetadata map[string]string
}
