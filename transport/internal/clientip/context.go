// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clientip

import (
	"context"
	"net/netip"

	corecontext "github.com/altessa-s/go-atlas/core/context"
)

type clientIpContextKey struct{}

// NewContext returns a child context carrying ip as the resolved client IP.
// A nil ctx is treated as [context.Background].
func NewContext(ctx context.Context, ip netip.Addr) context.Context {
	return context.WithValue(corecontext.OrBackground(ctx), clientIpContextKey{}, ip)
}

// FromContext extracts the client IP previously stored by [NewContext].
// Returns an invalid [netip.Addr] (zero value) when the context is nil or
// does not carry a client IP. Use Addr.IsValid to distinguish.
func FromContext(ctx context.Context) netip.Addr {
	ip, ok := corecontext.OrBackground(ctx).Value(clientIpContextKey{}).(netip.Addr)
	if !ok {
		return netip.Addr{}
	}
	return ip
}
