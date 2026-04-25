// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWrapAdapterError(t *testing.T) {
	err := WrapAdapterError(errors.New("fail"), "prometheus")
	require.Error(t, err)
}

func TestWrapMetricError(t *testing.T) {
	err := WrapMetricError(errors.New("fail"), "counter inc")
	require.Error(t, err)
}

func TestWrapFlushError(t *testing.T) {
	err := WrapFlushError(errors.New("fail"), "prometheus")
	require.Error(t, err)
}

func TestWrapShutdownError(t *testing.T) {
	err := WrapShutdownError(errors.New("fail"), "prometheus")
	require.Error(t, err)
}
