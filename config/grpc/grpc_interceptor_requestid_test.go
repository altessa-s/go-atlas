// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcconfig_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
)

func TestRequestIDInterceptorHeaderName(t *testing.T) {
	t.Parallel()

	cfg := grpcconfig.DefaultRequestIDInterceptor()
	require.Equal(t, "x-request-id", cfg.HeaderName)

	cfg.Enabled = true
	require.NoError(t, cfg.Validate())

	cfg.HeaderName = ""
	require.Error(t, cfg.Validate(), "an enabled interceptor needs a metadata key")
}
