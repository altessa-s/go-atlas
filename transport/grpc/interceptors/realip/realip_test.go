// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package realip

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/internal/clientip"
)

func TestServerInterceptor_Name(t *testing.T) {
	extractor, err := clientip.NewExtractor()
	require.NoError(t, err)
	i := ServerInterceptor(extractor)
	require.Equal(t, "realip", i.Name())
}

func TestServerInterceptor_ReturnsInterceptors(t *testing.T) {
	extractor, err := clientip.NewExtractor()
	require.NoError(t, err)
	i := ServerInterceptor(extractor)
	require.NotNil(t, i.ServerUnaryInterceptor(), "ServerUnaryInterceptor should not be nil")
	require.NotNil(t, i.ServerStreamInterceptor(), "ServerStreamInterceptor should not be nil")
}
