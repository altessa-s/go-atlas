// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metadata

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"
)

func TestInterceptor_Name(t *testing.T) {
	i := Interceptor()
	mi := i.(metadataInterceptor) //nolint:errcheck
	require.Equal(t, "metadata", mi.Name())
}

func TestInterceptor_Dependencies(t *testing.T) {
	i := Interceptor()
	mi := i.(metadataInterceptor) //nolint:errcheck
	require.Nil(t, mi.Dependencies())
}

func TestInterceptor_DrivenInterceptor(t *testing.T) {
	i := Interceptor()
	d, ctx := i.DrivenInterceptor(t.Context())
	require.NotNil(t, ctx, "context should not be nil")

	// Should return NoopDriver
	resp, err := d.PreCall(ctx, nil)
	require.Nil(t, resp)
	require.NoError(t, err)

	// Verify it implements the interface
	var _ driver.DrivenInterceptor = i
}
