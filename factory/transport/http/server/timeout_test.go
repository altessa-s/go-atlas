// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
)

func TestReadHeaderAndIdleTimeouts(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                         string
		header, idle                 time.Duration
		headerOverride, idleOverride *time.Duration
		wantHeader, wantIdle         time.Duration
	}{
		{name: "zero config", wantHeader: 10 * time.Second, wantIdle: 600 * time.Second},
		{name: "positive config", header: time.Second, idle: 2 * time.Second,
			wantHeader: time.Second, wantIdle: 2 * time.Second},
		{name: "zero overrides", header: time.Second, idle: 2 * time.Second,
			headerOverride: new(time.Duration(0)), idleOverride: new(time.Duration(0)),
			wantHeader: 10 * time.Second, wantIdle: 600 * time.Second},
		{name: "positive overrides", headerOverride: new(3 * time.Second), idleOverride: new(4 * time.Second),
			wantHeader: 3 * time.Second, wantIdle: 4 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := New(&httpconfig.Config{ReadHeaderTimeout: tc.header, IdleTimeout: tc.idle})
			if tc.headerOverride != nil {
				b.WithReadHeaderTimeout(*tc.headerOverride)
			}
			if tc.idleOverride != nil {
				b.WithIdleTimeout(*tc.idleOverride)
			}
			header, idle := b.readHeaderAndIdleTimeouts()
			require.Equal(t, tc.wantHeader, header)
			require.Equal(t, tc.wantIdle, idle)
		})
	}
}
