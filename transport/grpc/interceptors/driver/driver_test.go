// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package driver

import (
	"testing"
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
			if got := tt.st.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNoopDriver(t *testing.T) {
	d := NoopDriver()
	ctx := t.Context()

	resp, err := d.PreCall(ctx, "req")
	if resp != nil {
		t.Fatal("PreCall resp should be nil")
	}
	if err != nil {
		t.Fatal("PreCall err should be nil")
	}

	err = d.PostCall(ctx, "resp", nil)
	if err != nil {
		t.Fatal("PostCall err should be nil")
	}
}

func BenchmarkNoopDriver_PreCall(b *testing.B) {
	d := NoopDriver()
	ctx := b.Context()
	for b.Loop() {
		d.PreCall(ctx, nil) //nolint:errcheck
	}
}
