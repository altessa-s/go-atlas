// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatusCode_String(t *testing.T) {
	tests := []struct {
		code StatusCode
		want string
	}{
		{StatusUnset, "Unset"},
		{StatusOK, "OK"},
		{StatusError, "Error"},
		{StatusCode(99), "Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			require.Equal(t, tt.want, tt.code.String())
		})
	}
}

func TestTraceFlags_IsSampled(t *testing.T) {
	tests := []struct {
		name  string
		flags TraceFlags
		want  bool
	}{
		{"sampled", FlagsSampled, true},
		{"not sampled", 0, false},
		{"with extra bits", TraceFlags(0x03), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.flags.IsSampled())
		})
	}
}

func TestTraceFlags_WithSampled(t *testing.T) {
	tests := []struct {
		name    string
		flags   TraceFlags
		sampled bool
		want    TraceFlags
	}{
		{"set sampled", 0, true, FlagsSampled},
		{"clear sampled", FlagsSampled, false, 0},
		{"already set", FlagsSampled, true, FlagsSampled},
		{"preserve other bits", TraceFlags(0x02), true, TraceFlags(0x03)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.flags.WithSampled(tt.sampled))
		})
	}
}

func TestTraceFlags_String(t *testing.T) {
	tests := []struct {
		flags TraceFlags
		want  string
	}{
		{0, "00"},
		{FlagsSampled, "01"},
		{TraceFlags(0xff), "ff"},
		{TraceFlags(0x0a), "0a"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			require.Equal(t, tt.want, tt.flags.String())
		})
	}
}
