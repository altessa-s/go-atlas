// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDefaultSuccessOnlyDecision(t *testing.T) {
	fn := DefaultSuccessOnlyDecision(5 * time.Minute)
	ctx := t.Context()

	tests := []struct {
		name        string
		resp        any
		err         error
		shouldCache bool
	}{
		{"success", "resp", nil, true},
		{"error", nil, status.Error(codes.NotFound, "nf"), false},
		{"nil_resp", nil, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := fn(ctx, "/svc/Method", nil, tt.resp, tt.err)
			require.Equal(t, d.ShouldCache, tt.shouldCache)
			require.False(t, tt.shouldCache && d.TTL != 5*time.Minute)
		})
	}
}

func TestDefaultSuccessAndErrorDecision(t *testing.T) {
	fn := DefaultSuccessAndErrorDecision(5*time.Minute, 1*time.Minute)
	ctx := t.Context()

	tests := []struct {
		name        string
		resp        any
		err         error
		shouldCache bool
		ttl         time.Duration
	}{
		{"success", "resp", nil, true, 5 * time.Minute},
		{"not_found", nil, status.Error(codes.NotFound, "nf"), true, 1 * time.Minute},
		{"already_exists", nil, status.Error(codes.AlreadyExists, "ae"), true, 1 * time.Minute},
		{"internal", nil, status.Error(codes.Internal, "err"), false, 0},
		{"nil_resp_no_err", nil, nil, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := fn(ctx, "/svc/Method", nil, tt.resp, tt.err)
			require.Equal(t, d.ShouldCache, tt.shouldCache)
			require.False(t, tt.shouldCache && d.TTL != tt.ttl)
		})
	}
}

func BenchmarkDefaultSuccessOnlyDecision(b *testing.B) {
	fn := DefaultSuccessOnlyDecision(5 * time.Minute)
	ctx := b.Context()
	for b.Loop() {
		fn(ctx, "/svc/Method", nil, "resp", nil)
	}
}
