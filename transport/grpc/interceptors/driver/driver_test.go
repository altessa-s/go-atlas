// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package driver

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamType_String(t *testing.T) {
	tests := []struct {
		name string
		st   StreamType
		want string
	}{
		{"none", StreamTypeNone, "none"},
		{"client", StreamTypeClient, "client"},
		{"server", StreamTypeServer, "server"},
		{"bidi", StreamTypeBidi, "bidi"},
		{"unknown", StreamType(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.st.String()
			require.Equal(t, tt.want, got)
		})
	}
}

func TestNoopDriver(t *testing.T) {
	d := NoopDriver()
	ctx := t.Context()

	resp, err := d.PreCall(ctx, "req")
	require.Nil(t, resp)
	require.NoError(t, err)

	err = d.PostCall(ctx, "resp", nil)
	require.NoError(t, err)
}

func BenchmarkNoopDriver_PreCall(b *testing.B) {
	d := NoopDriver()
	ctx := b.Context()
	for b.Loop() {
		d.PreCall(ctx, nil) //nolint:errcheck
	}
}
