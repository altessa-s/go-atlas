// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcconfig_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
)

func TestMetricsInterceptor_MaxMethods(t *testing.T) {
	t.Parallel()

	m := grpcconfig.DefaultMetricsInterceptor()
	m.Enabled = true
	require.Equal(t, 1024, m.MaxMethods)

	m.MaxMethods = 0
	require.NoError(t, m.Validate(), "zero keeps the interceptor default")

	m.MaxMethods = -1
	require.Contains(t, fieldErrors(t, m.Validate()), "MaxMethods")
}
