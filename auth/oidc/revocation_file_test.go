// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type cancelDuringCount struct {
	context.Context
	cancel    context.CancelFunc
	remaining int
}

func (c *cancelDuringCount) Err() error {
	c.remaining--
	if c.remaining == 0 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestFileLoaderCancellation(t *testing.T) {
	t.Parallel()
	file := &FileRevocationLoader{Path: filepath.Join(t.TempDir(), "entries")}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := file.Count(ctx)
	require.ErrorIs(t, err, context.Canceled)
	var streamErr error
	for _, err := range file.StreamValues(ctx) {
		streamErr = err
	}
	require.ErrorIs(t, streamErr, context.Canceled)
	require.NoError(t, os.WriteFile(file.Path, []byte(strings.Repeat("item\n", 1<<16)), 0600))
	mid, stop := context.WithCancel(t.Context())
	defer stop()
	_, err = file.Count(&cancelDuringCount{Context: mid, cancel: stop, remaining: 128})
	require.ErrorIs(t, err, context.Canceled)
}
