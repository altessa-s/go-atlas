// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisconfig_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	redisconfig "github.com/altessa-s/go-atlas/config/redis"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func TestDefault(t *testing.T) {
	t.Parallel()

	c := redisconfig.Default()
	require.Equal(t, []string{"localhost:6379"}, c.Hosts)
	require.False(t, c.UseConnectionURI())
	require.NoError(t, c.Validate())
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	t.Run("hosts required without connection URI", func(t *testing.T) {
		t.Parallel()
		c := redisconfig.Default()
		c.Hosts = nil
		verrs, ok := errors.AsType[validation.Errors](c.Validate())
		require.True(t, ok)
		require.Contains(t, verrs, "Hosts")
	})

	t.Run("socket timeout must be positive", func(t *testing.T) {
		t.Parallel()
		c := redisconfig.Default()
		c.SocketTimeout = 0
		verrs, ok := errors.AsType[validation.Errors](c.Validate())
		require.True(t, ok)
		require.Contains(t, verrs, "SocketTimeout")
	})

	t.Run("empty keys prefix pointer rejected", func(t *testing.T) {
		t.Parallel()
		c := redisconfig.Default()
		c.KeysPrefix = new("")
		verrs, ok := errors.AsType[validation.Errors](c.Validate())
		require.True(t, ok)
		require.Contains(t, verrs, "KeysPrefix")
	})

	t.Run("connection URI replaces hosts", func(t *testing.T) {
		t.Parallel()
		c := redisconfig.Default()
		c.Hosts = nil
		c.ConnectionURI = "redis://localhost:6379/0"
		require.True(t, c.UseConnectionURI())
		require.NoError(t, c.Validate())
	})

	t.Run("connection URI conflicts with credentials", func(t *testing.T) {
		t.Parallel()
		c := redisconfig.Default()
		c.ConnectionURI = "redis://localhost:6379/0"
		c.Username = "u"
		c.Password = "p"
		err := c.Validate()
		require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
		require.ErrorContains(t, err, "username must not be set")
		require.ErrorContains(t, err, "password must not be set")
	})
}
