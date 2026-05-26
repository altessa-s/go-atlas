// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/health"
)

func TestNew_Default(t *testing.T) {
	b := New(nil)
	require.NotNil(t, b)
}

func TestNew_WithOptions(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	coord := health.New()
	defer coord.Close()

	b := New(&config.Meilisearch{}).
		UseLogger(logger).
		UseHealthCoordinator(coord)
	require.NotNil(t, b)
}

func TestNew_NilOptions(t *testing.T) {
	b := New(nil).
		UseLogger(nil).
		UseHealthCoordinator(nil)
	require.NotNil(t, b)
}
