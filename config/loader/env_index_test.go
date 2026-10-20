// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader"
)

// All tests in this file mutate process environment via t.Setenv, which is
// incompatible with t.Parallel — keep them serial.

type indexItem struct {
	Name string   `yaml:"name"`
	Tags []string `yaml:"tags"`
}

type indexConfig struct {
	Hosts  []string            `yaml:"hosts"`
	Items  []indexItem         `yaml:"items"`
	Groups map[string][]string `yaml:"groups"`
}

// TestLoad_EnvSliceIndexRejected checks that an index the environment cannot
// address fails the load instead of panicking or allocating without bound.
func TestLoad_EnvSliceIndexRejected(t *testing.T) {
	maxInt := strconv.Itoa(math.MaxInt)
	tests := []struct {
		name string
		envs map[string]string
	}{
		{name: "huge", envs: map[string]string{"ENVIDX_HOSTS__1000000000": "x"}},
		{name: "max int", envs: map[string]string{"ENVIDX_HOSTS__" + maxInt: "x"}},
		{name: "negative", envs: map[string]string{"ENVIDX_HOSTS__-1": "x"}},
		{name: "negative mixed with zero", envs: map[string]string{"ENVIDX_HOSTS__0": "a", "ENVIDX_HOSTS__-1": "b"}},
		{name: "struct element huge", envs: map[string]string{"ENVIDX_ITEMS__1000000000__NAME": "x"}},
		{name: "struct element max int", envs: map[string]string{"ENVIDX_ITEMS__" + maxInt + "__NAME": "x"}},
		{name: "struct element negative", envs: map[string]string{"ENVIDX_ITEMS__0__NAME": "a", "ENVIDX_ITEMS__-1__NAME": "b"}},
		{name: "nested slice huge", envs: map[string]string{"ENVIDX_ITEMS__0__TAGS__1000000000": "x"}},
		{name: "map value max int", envs: map[string]string{"ENVIDX_GROUPS__east__" + maxInt: "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envs {
				t.Setenv(k, v)
			}

			_, err := loader.New(nil, loader.WithEnvPrefix("ENVIDX")).Load(&indexConfig{})
			require.ErrorIs(t, err, loader.ErrEnvIndexOutOfRange)
			require.ErrorIs(t, err, loader.ErrBindEnv)
		})
	}
}

// TestLoad_EnvSliceIndexLimit checks that the limit bounds how far the
// environment grows a slice, while indexes inside an existing longer slice
// stay addressable.
func TestLoad_EnvSliceIndexLimit(t *testing.T) {
	t.Setenv("ENVIDX_HOSTS__1", "h1")
	t.Setenv("ENVIDX_ITEMS__3__NAME", "i3")

	cfg := &indexConfig{Items: make([]indexItem, 4)}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVIDX"), loader.WithMaxEnvSliceLen(2)).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, []string{"", "h1"}, cfg.Hosts)
	require.Equal(t, "i3", cfg.Items[3].Name)

	t.Setenv("ENVIDX_HOSTS__2", "h2")
	_, err = loader.New(nil, loader.WithEnvPrefix("ENVIDX"), loader.WithMaxEnvSliceLen(2)).Load(&indexConfig{})
	require.ErrorIs(t, err, loader.ErrEnvIndexOutOfRange)
}

// TestLoad_EnvSliceIndexDefaultLimit checks the default limit: the last
// index below it is accepted.
func TestLoad_EnvSliceIndexDefaultLimit(t *testing.T) {
	t.Setenv("ENVIDX_HOSTS__"+strconv.Itoa(loader.DefaultMaxEnvSliceLen-1), "last")

	cfg := &indexConfig{}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVIDX")).Load(cfg)
	require.NoError(t, err)
	require.Len(t, cfg.Hosts, loader.DefaultMaxEnvSliceLen)
	require.Equal(t, "last", cfg.Hosts[loader.DefaultMaxEnvSliceLen-1])
}
