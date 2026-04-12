// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWrapCheckError(t *testing.T) {
	err := WrapCheckError(errors.New("ping failed"), "redis")
	require.Error(t, err)
	require.NotEmpty(t, err.Error())
}

func TestWrapWatchError(t *testing.T) {
	err := WrapWatchError(errors.New("watch failed"), "svc")
	require.Error(t, err)
}

func TestWrapShutdownError(t *testing.T) {
	err := WrapShutdownError(errors.New("shutdown failed"))
	require.Error(t, err)
}
