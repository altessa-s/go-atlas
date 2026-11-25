// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/lifecycle"
)

func TestLifecycleTimeoutsTakePrecedence(t *testing.T) {
	t.Parallel()
	want := lifecycle.DefaultTimeouts()
	want.StartupVerification = time.Millisecond
	for _, opts := range [][]Option{
		{WithStartupVerification(0), WithLifecycleTimeouts(want)},
		{WithLifecycleTimeouts(want), WithStartupVerification(0)},
	} {
		srv, err := New(opts...)
		require.NoError(t, err)
		require.Equal(t, want, srv.LifecycleTimeouts())
	}
}
