// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
)

func TestSecrets_EmptyListingThreshold(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultSecrets()
	require.Equal(t, 3, cfg.EmptyListingThreshold)
	require.False(t, cfg.AllowShallowClone)
	require.NoError(t, cfg.Validate())

	cfg.EmptyListingThreshold = 0 // manager default
	require.NoError(t, cfg.Validate())

	cfg.EmptyListingThreshold = -1
	require.Error(t, cfg.Validate())
}
