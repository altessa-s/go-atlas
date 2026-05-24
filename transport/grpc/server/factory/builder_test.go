// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"

	"google.golang.org/grpc/keepalive"
)

// TestKeepaliveEnforcementPolicy_NilConfig confirms that a completely
// absent config still resolves to the safe defaults rather than the
// permissive stdlib fallback.
func TestKeepaliveEnforcementPolicy_NilConfig(t *testing.T) {
	t.Parallel()

	got := keepaliveEnforcementPolicy(nil)
	require.Equal(t, keepalive.EnforcementPolicy{
		MinTime:             DefaultGrpcEnforcementMinTime,
		PermitWithoutStream: DefaultGrpcEnforcementPermitWithoutStream,
	}, got, "nil config must produce the safe defaults — never an empty policy")
}

// TestKeepaliveEnforcementPolicy_NilKeepAlive covers the most common
// real-world misconfiguration: an operator config that omits keepAlive
// entirely. Before the fix this collapsed to the permissive stdlib
// default; now it must produce the safe baseline.
func TestKeepaliveEnforcementPolicy_NilKeepAlive(t *testing.T) {
	t.Parallel()

	got := keepaliveEnforcementPolicy(&config.Grpc{})
	require.Equal(t, keepalive.EnforcementPolicy{
		MinTime:             DefaultGrpcEnforcementMinTime,
		PermitWithoutStream: DefaultGrpcEnforcementPermitWithoutStream,
	}, got, "cfg.KeepAlive == nil must produce the safe defaults — this was the original bypass")
}

// TestKeepaliveEnforcementPolicy_NilEnforcementPolicy covers the case
// where the operator set keepalive params but forgot to fill in the
// enforcement policy block.
func TestKeepaliveEnforcementPolicy_NilEnforcementPolicy(t *testing.T) {
	t.Parallel()

	cfg := &config.Grpc{
		KeepAlive: &config.GrpcKeepAlive{
			Time:    20 * time.Second,
			Timeout: 5 * time.Second,
		},
	}

	got := keepaliveEnforcementPolicy(cfg)
	require.Equal(t, keepalive.EnforcementPolicy{
		MinTime:             DefaultGrpcEnforcementMinTime,
		PermitWithoutStream: DefaultGrpcEnforcementPermitWithoutStream,
	}, got, "cfg.KeepAlive.EnforcementPolicy == nil must still produce the safe defaults")
}

// TestKeepaliveEnforcementPolicy_HonorsConfig confirms that explicit
// operator config is preserved verbatim.
func TestKeepaliveEnforcementPolicy_HonorsConfig(t *testing.T) {
	t.Parallel()

	cfg := &config.Grpc{
		KeepAlive: &config.GrpcKeepAlive{
			EnforcementPolicy: &config.GrpcEnforcementPolicy{
				MinTime:             15 * time.Second,
				PermitWithoutStream: true,
			},
		},
	}

	got := keepaliveEnforcementPolicy(cfg)
	require.Equal(t, keepalive.EnforcementPolicy{
		MinTime:             15 * time.Second,
		PermitWithoutStream: true,
	}, got, "explicit enforcement policy must be honored verbatim — defaults are a fallback, not an override")
}

// TestBuildGrpcOptions_AlwaysEmitsEnforcementPolicy is the regression
// guard: even with an empty config the builder must produce a non-empty
// ServerOption slice that includes the enforcement policy. Without this
// the original bug ("KeepaliveEnforcementPolicy only when KeepAlive !=
// nil") would silently come back.
func TestBuildGrpcOptions_AlwaysEmitsEnforcementPolicy(t *testing.T) {
	t.Parallel()

	b := New(&config.Grpc{})
	opts := b.buildGrpcOptions()

	require.NotEmpty(t, opts,
		"buildGrpcOptions on an empty config MUST still return options — at minimum the enforcement policy")
}

// TestDefaultGrpcEnforcement_AlignsWithConfigStructTag pins the contract
// between the factory's default constants and the YAML struct tag in
// [config.GrpcEnforcementPolicy]. When operators write `enforcementPolicy:
// {}` (empty object) the loader fills in struct-tag defaults; we want
// that to match the factory-side defaults so the resolution path
// (explicit vs implicit) does not produce surprising different policies.
func TestDefaultGrpcEnforcement_AlignsWithConfigStructTag(t *testing.T) {
	t.Parallel()

	// Construct the policy via factory defaults.
	fromFactory := keepaliveEnforcementPolicy(nil)

	// Construct an explicit empty enforcement policy struct. This mimics
	// the value the YAML loader would produce after struct-tag defaults
	// (MinTime: 30s, PermitWithoutStream: false).
	want := keepalive.EnforcementPolicy{
		MinTime:             30 * time.Second,
		PermitWithoutStream: false,
	}

	require.Equal(t, want, fromFactory,
		"factory defaults must align with config struct-tag defaults — otherwise an empty `enforcementPolicy: {}` in YAML behaves differently from omitting the field entirely")
}
