// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisutils_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/internal/redisutils"
)

func TestNewKeyBuilder(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		want   string
	}{
		{"with prefix", "myapp:cache", "myapp:cache"},
		{"with trailing separator", "myapp:cache:", "myapp:cache"},
		{"empty prefix", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kb := redisutils.NewKeyBuilder(tt.prefix)
			require.Equal(t, tt.want, kb.Prefix())
		})
	}
}

func TestNewKeyBuilderWithSeparator(t *testing.T) {
	kb := redisutils.NewKeyBuilderWithSeparator("app|ns|", "|")
	require.Equal(t, "app|ns", kb.Prefix())
}

func TestKeyBuilder_Build(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		key    string
		want   string
	}{
		{"with prefix", "myapp", "user:123", "myapp:user:123"},
		{"empty prefix", "", "user:123", "user:123"},
		{"empty key", "myapp", "", "myapp:"},
		{"both empty", "", "", ""},
		{"trailing colon prefix", "myapp:", "key", "myapp:key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kb := redisutils.NewKeyBuilder(tt.prefix)
			require.Equal(t, tt.want, kb.Build(tt.key))
		})
	}
}

func TestKeyBuilder_BuildMany(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		keys   []string
		want   []string
	}{
		{"with prefix", "app", []string{"k1", "k2"}, []string{"app:k1", "app:k2"}},
		{"empty prefix", "", []string{"k1", "k2"}, []string{"k1", "k2"}},
		{"empty keys", "app", []string{}, []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kb := redisutils.NewKeyBuilder(tt.prefix)
			got := kb.BuildMany(tt.keys)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestKeyBuilder_BuildMany_DoesNotModifyOriginal(t *testing.T) {
	kb := redisutils.NewKeyBuilder("")
	original := []string{"a", "b"}
	result := kb.BuildMany(original)
	result[0] = "modified"
	require.Equal(t, "a", original[0], "BuildMany modified original slice")
}

func TestKeyBuilder_Pattern(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		want   string
	}{
		{"with prefix", "myapp", "myapp:*"},
		{"empty prefix", "", "*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kb := redisutils.NewKeyBuilder(tt.prefix)
			require.Equal(t, tt.want, kb.Pattern())
		})
	}
}

func TestKeyBuilder_HasPrefix(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		want   bool
	}{
		{"with prefix", "myapp", true},
		{"empty prefix", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kb := redisutils.NewKeyBuilder(tt.prefix)
			require.Equal(t, tt.want, kb.HasPrefix())
		})
	}
}
