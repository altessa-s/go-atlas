// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package serviceinfo

import (
	"context"

	_ "github.com/altessa-s/go-atlas/core/runtime/appinfo"
)

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

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

// leaderStater is the context-aware shape of [LeaderProvider]: it answers
// both leadership questions in a single call that receives the caller's
// context.
//
// It is deliberately separate from LeaderProvider, and unexported, so existing
// implementations stay source-compatible — the same reasoning as
// providers.Prober in data/locks/dlock. [Handler.Snapshot] type-asserts for it
// and falls back to the two-method path when it is absent.
//
// It exists because LeaderProvider.IsLeader takes no context. For an
// implementation whose leadership check is a local read — *leadelect.Leader is
// one, it reads an atomic — that is exactly right. For one that has to ask the
// network, a context-free signature leaves no way to pass the request's
// deadline or cancellation, so the call outlives the client that asked for it.
type leaderStater interface {
	LeaderState(ctx context.Context) (isLeader bool, leaderID string)
}

// LeaderFunc is the function shape accepted by [WithLeader] for callers
// that don't have a [LeaderProvider]-shaped object. It is invoked once
// per snapshot — keep it cheap. Returning a non-nil error from the
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

// leaderFnAdapter satisfies [LeaderProvider] and [leaderStater] by invoking
// the wrapped function.
//
// [Handler.Snapshot] goes through LeaderState, so fn is called once per
// snapshot and receives the request's context — which is what LeaderFunc's
// signature promises. The two-method path below remains for any other caller
// that holds this adapter as a plain LeaderProvider.
type leaderFnAdapter struct {
	fn LeaderFunc
}

func (a leaderFnAdapter) LeaderState(ctx context.Context) (bool, string) {
	return a.fn(ctx)
}

// IsLeader satisfies [LeaderProvider], whose signature carries no context.
// Prefer LeaderState, which does; this exists so the adapter still fits the
// plain interface.
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
	leaderProvider LeaderProvider `optgen:"notnil" optval:"nil"`

	// extraMetadata is merged on top of the runtime/build metadata
	// populated by [appinfo]. Conflicting keys from the caller win.
	// To surface the version of a proto module, pass it here under a
	// "proto_version" key — the handler intentionally does not look up
	// dep versions on the caller's behalf.
	extraMetadata map[string]string
}
