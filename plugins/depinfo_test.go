// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"bytes"
	"log/slog"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDepInfo(t *testing.T) {
	t.Parallel()

	di := &DepInfo{
		GoVersion: "go1.25.0",
		Deps:      []ModDep{{Path: "example.com/a", Version: "v1.0.0"}},
	}
	diPtr := &di

	tests := []struct {
		name    string
		symbols map[string]any
		wantNil bool
		wantErr error
		wantFn  func(t *testing.T, got *DepInfo)
	}{
		{
			name:    "value form (*DepInfo)",
			symbols: map[string]any{"DepInfo": di},
			wantFn: func(t *testing.T, got *DepInfo) {
				assert.Equal(t, "go1.25.0", got.GoVersion)
				require.Len(t, got.Deps, 1)
				assert.Equal(t, "example.com/a", got.Deps[0].Path)
			},
		},
		{
			name:    "pointer form (**DepInfo)",
			symbols: map[string]any{"DepInfo": diPtr},
			wantFn: func(t *testing.T, got *DepInfo) {
				assert.Equal(t, "go1.25.0", got.GoVersion)
			},
		},
		{
			name:    "absent symbol returns (nil, nil)",
			symbols: map[string]any{},
			wantNil: true,
		},
		{
			name:    "nil *DepInfo returns (nil, nil)",
			symbols: map[string]any{"DepInfo": (*DepInfo)(nil)},
			wantNil: true,
		},
		{
			name:    "nil **DepInfo returns (nil, nil)",
			symbols: map[string]any{"DepInfo": (**DepInfo)(nil)},
			wantNil: true,
		},
		{
			name:    "zero-value DepInfo is valid",
			symbols: map[string]any{"DepInfo": &DepInfo{}},
			wantFn: func(t *testing.T, got *DepInfo) {
				assert.Empty(t, got.GoVersion)
				assert.Empty(t, got.Deps)
			},
		},
		{
			name:    "wrong type returns ErrInvalidDepInfo",
			symbols: map[string]any{"DepInfo": "not a DepInfo"},
			wantErr: ErrInvalidDepInfo,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveDepInfo(stubLookup(tc.symbols))
			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			if tc.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			tc.wantFn(t, got)
		})
	}
}

func TestCheckDepInfo(t *testing.T) {
	t.Parallel()

	hostBI := func() *debug.BuildInfo {
		return &debug.BuildInfo{
			GoVersion: "go1.25.0",
			Deps: []*debug.Module{
				{Path: "example.com/shared", Version: "v1.2.0"},
				{Path: "example.com/other", Version: "v3.0.0"},
			},
		}
	}

	tests := []struct {
		name       string
		pluginDeps *DepInfo
		hostBI     hostBuildInfo
		wantWarn   bool
		wantInLog  []string // additional substrings that must appear in the log
	}{
		{
			name:       "nil DepInfo is no-op",
			pluginDeps: nil,
			hostBI:     hostBI,
		},
		{
			name:       "nil host build info is no-op",
			pluginDeps: &DepInfo{Deps: []ModDep{{Path: "example.com/shared", Version: "v9.9.9"}}},
			hostBI:     func() *debug.BuildInfo { return nil },
		},
		{
			name: "matching versions produce no warning",
			pluginDeps: &DepInfo{
				Deps: []ModDep{
					{Path: "example.com/shared", Version: "v1.2.0"},
				},
			},
			hostBI: hostBI,
		},
		{
			name: "plugin-only dep produces no warning",
			pluginDeps: &DepInfo{
				Deps: []ModDep{
					{Path: "example.com/plugin-only", Version: "v1.0.0"},
				},
			},
			hostBI: hostBI,
		},
		{
			name: "single version mismatch produces warning",
			pluginDeps: &DepInfo{
				Deps: []ModDep{
					{Path: "example.com/shared", Version: "v1.1.0"},
					{Path: "example.com/other", Version: "v3.0.0"},
				},
			},
			hostBI:    hostBI,
			wantWarn:  true,
			wantInLog: []string{"example.com/shared"},
		},
		{
			name: "multiple mismatches all reported",
			pluginDeps: &DepInfo{
				Deps: []ModDep{
					{Path: "example.com/shared", Version: "v1.1.0"},
					{Path: "example.com/other", Version: "v3.1.0"},
				},
			},
			hostBI:    hostBI,
			wantWarn:  true,
			wantInLog: []string{"example.com/shared", "example.com/other"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

			checkDepInfo(logger, "test-plugin", tc.pluginDeps, tc.hostBI)

			if tc.wantWarn {
				assert.Contains(t, buf.String(), "mismatches")
				for _, want := range tc.wantInLog {
					assert.Contains(t, buf.String(), want)
				}
			} else {
				assert.NotContains(t, buf.String(), "mismatches")
			}
		})
	}
}

func TestCheckDepInfo_HostReplacedModule(t *testing.T) {
	t.Parallel()

	hostBI := func() *debug.BuildInfo {
		return &debug.BuildInfo{
			Deps: []*debug.Module{
				{
					Path:    "example.com/original",
					Version: "v1.0.0",
					Replace: &debug.Module{
						Path:    "example.com/fork",
						Version: "v1.0.0-fork",
					},
				},
			},
		}
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	// Plugin has the fork at a different version → should warn.
	pluginDeps := &DepInfo{
		Deps: []ModDep{{Path: "example.com/fork", Version: "v1.0.0-other"}},
	}
	checkDepInfo(logger, "test-plugin", pluginDeps, hostBI)
	assert.Contains(t, buf.String(), "mismatches")
}

func TestNewDepInfoFromBuild(t *testing.T) {
	// NewDepInfoFromBuild reads the test binary's own build info.
	// Under `go test` in module mode this should be non-nil.
	di := NewDepInfoFromBuild()
	if di == nil {
		t.Skip("debug.ReadBuildInfo unavailable in this test environment")
	}
	assert.NotEmpty(t, di.GoVersion)
}
