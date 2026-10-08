// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
)

func TestLoggerMiddleware_StatusLevels(t *testing.T) {
	t.Parallel()

	cfg := func(levels map[int]string) *LoggerMiddleware {
		c := DefaultLoggerMiddleware()
		c.EnableMixin = middlewareconfig.EnableMixin{Enabled: true}
		c.StatusLevels = levels
		return &c
	}

	c := cfg(map[int]string{401: "info", 404: "DEBUG"})
	require.NoError(t, c.Validate())
	levels, err := c.SlogStatusLevels()
	require.NoError(t, err)
	require.Equal(t, map[int]slog.Level{401: slog.LevelInfo, 404: slog.LevelDebug}, levels)

	none, err := cfg(nil).SlogStatusLevels()
	require.NoError(t, err)
	require.Nil(t, none)

	require.Error(t, cfg(map[int]string{401: "loud"}).Validate(), "unknown level")
	require.Error(t, cfg(map[int]string{42: "info"}).Validate(), "status out of range")
}
