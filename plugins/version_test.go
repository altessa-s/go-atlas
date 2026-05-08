// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMajorOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "empty", version: "", want: ""},
		{name: "bare major", version: "1", want: "1"},
		{name: "v-prefixed bare", version: "v2", want: "2"},
		{name: "full semver", version: "1.2.3", want: "1"},
		{name: "v-prefixed semver", version: "v3.4.5", want: "3"},
		{name: "prerelease", version: "v2.0.0-rc.1", want: "2"},
		{name: "build metadata", version: "3.0.0+build.5", want: "3"},
		{name: "pseudo-version", version: "v0.0.0-20240101000000-abcdef", want: "0"},
		{name: "non-numeric major preserved", version: "abc", want: "abc"},
		{name: "leading dot", version: ".1.2", want: ""},
		{name: "double v", version: "vv1", want: "v1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, majorOf(tc.version))
		})
	}
}

func TestManager_checkHostMajor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mode        HostVersionMode
		hostVersion string
		descVersion string
		wantErr     error
		wantWarnLog bool
	}{
		{
			name:        "enforce match",
			mode:        HostVersionEnforce,
			hostVersion: "2.5.3",
			descVersion: "2.1.0",
		},
		{
			name:        "enforce mismatch",
			mode:        HostVersionEnforce,
			hostVersion: "3.0.0",
			descVersion: "2.0.0",
			wantErr:     ErrHostVersionMismatch,
		},
		{
			name:        "enforce v-prefix mismatch",
			mode:        HostVersionEnforce,
			hostVersion: "v3.0.0",
			descVersion: "v2.0.0-rc.1",
			wantErr:     ErrHostVersionMismatch,
		},
		{
			name:        "enforce host empty skips",
			mode:        HostVersionEnforce,
			hostVersion: "",
			descVersion: "2.0.0",
		},
		{
			name:        "enforce desc empty skips (backward compat)",
			mode:        HostVersionEnforce,
			hostVersion: "2.0.0",
			descVersion: "",
		},
		{
			name:        "warn match no log",
			mode:        HostVersionWarn,
			hostVersion: "2.0.0",
			descVersion: "2.5.0",
		},
		{
			name:        "warn mismatch logs and loads",
			mode:        HostVersionWarn,
			hostVersion: "3.0.0",
			descVersion: "2.0.0",
			wantWarnLog: true,
		},
		{
			name:        "disabled mismatch ignored",
			mode:        HostVersionDisabled,
			hostVersion: "3.0.0",
			descVersion: "2.0.0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			mgr := NewManager(
				WithLogger(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))),
				WithHostVersion(tc.hostVersion),
				WithHostVersionMode(tc.mode),
			)
			t.Cleanup(func() { _ = mgr.Close() })

			err := mgr.checkHostMajor(&Descriptor{Name: "p", HostVersion: tc.descVersion})

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tc.wantErr), "want %v, got %v", tc.wantErr, err)
			} else {
				require.NoError(t, err)
			}

			if tc.wantWarnLog {
				assert.Contains(t, buf.String(), "plugin host major version mismatch")
			} else {
				assert.NotContains(t, buf.String(), "plugin host major version mismatch")
			}
		})
	}
}

func TestManager_checkHostMajor_ErrorMessageIncludesVersions(t *testing.T) {
	t.Parallel()

	mgr := NewManager(
		WithHostVersion("3.1.0"),
		WithHostVersionMode(HostVersionEnforce),
	)
	t.Cleanup(func() { _ = mgr.Close() })

	err := mgr.checkHostMajor(&Descriptor{Name: "p", HostVersion: "2.0.0-rc.1"})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrHostVersionMismatch))
	msg := err.Error()
	assert.True(t, strings.Contains(msg, `"2"`) && strings.Contains(msg, `"3"`),
		"error should report both majors, got: %s", msg)
	assert.Contains(t, msg, "2.0.0-rc.1", "error should include full plugin version")
	assert.Contains(t, msg, "3.1.0", "error should include full host version")
}
