// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrorf(t *testing.T) {
	t.Parallel()

	cause := errors.New("root cause")
	err := Errorf(ErrUnsupported, "unknown storage type %q: %w", "etcd", cause)

	require.Equal(t, `unknown storage type "etcd": root cause`, err.Error())
	require.ErrorIs(t, err, ErrUnsupported)
	require.ErrorIs(t, err, cause)
	require.NotErrorIs(t, err, ErrNotRegistered)
}

func TestErrorf_SurvivesWrapping(t *testing.T) {
	t.Parallel()

	b := NewBase(nil)
	err := b.WrapError(Errorf(ErrNotRegistered, "storage %q is not registered", "redis"), "build")

	require.ErrorIs(t, err, ErrNotRegistered)
}

func TestRequireDependency_MatchesSentinel(t *testing.T) {
	t.Parallel()

	b := NewBase(nil)
	require.ErrorIs(t, b.RequireDependency(nil, "dep"), ErrDependencyRequired)
	require.ErrorIs(t, b.RequireAllDependencies(map[string]any{"a": nil, "b": nil}), ErrDependencyRequired)
}
