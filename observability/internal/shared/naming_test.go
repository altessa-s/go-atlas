// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package shared

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildName(t *testing.T) {
	tests := []struct {
		name      string
		parts     []string
		separator byte
		want      string
	}{
		{"full", []string{"myapp", "http", "requests"}, '_', "myapp_http_requests"},
		{"empty middle", []string{"myapp", "", "requests"}, '_', "myapp_requests"},
		{"single", []string{"requests"}, '_', "requests"},
		{"all empty", []string{"", "", ""}, '_', ""},
		{"nil", nil, '_', ""},
		{"slash separator", []string{"scope", "name"}, '/', "scope/name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, BuildName(tt.parts, tt.separator))
		})
	}
}

func TestBuildMetricName(t *testing.T) {
	tests := []struct {
		name                  string
		service, sub, metName string
		want                  string
	}{
		{"full", "app", "http", "requests", "app_http_requests"},
		{"no subsystem", "app", "", "requests", "app_requests"},
		{"no service", "", "http", "requests", "http_requests"},
		{"name only", "", "", "requests", "requests"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, BuildMetricName(tt.service, tt.sub, tt.metName))
		})
	}
}

func TestBuildTracerName(t *testing.T) {
	tests := []struct {
		name         string
		scope, tName string
		want         string
	}{
		{"full", "myapp", "orders", "myapp/orders"},
		{"no scope", "", "orders", "orders"},
		{"no name", "myapp", "", "myapp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, BuildTracerName(tt.scope, tt.tName))
		})
	}
}

func TestJoinScope(t *testing.T) {
	tests := []struct {
		name          string
		parent, child string
		sep           byte
		want          string
	}{
		{"both", "a", "b", '_', "a_b"},
		{"empty parent", "", "b", '_', "b"},
		{"empty child", "a", "", '_', "a"},
		{"both empty", "", "", '_', ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, JoinScope(tt.parent, tt.child, tt.sep))
		})
	}
}

func TestJoinMetricScope(t *testing.T) {
	require.Equal(t, "parent_child", JoinMetricScope("parent", "child"))
}

func TestJoinTracerScope(t *testing.T) {
	require.Equal(t, "parent/child", JoinTracerScope("parent", "child"))
}
