// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"

	"google.golang.org/grpc/metadata"
)

func TestProcessorWithoutIncomingMetadata(t *testing.T) {
	t.Parallel()
	type tenantKey struct{}
	gen := cache.NewKeyGenerator(nil, func(ctx context.Context, md metadata.MD) map[string]string {
		tenant, ok := ctx.Value(tenantKey{}).(string)
		require.True(t, ok)
		require.NotNil(t, md)
		return map[string]string{"tenant": tenant}
	})
	a, err := gen(context.WithValue(t.Context(), tenantKey{}, "a"), "/svc/Get", "request")
	require.NoError(t, err)
	b, err := gen(context.WithValue(t.Context(), tenantKey{}, "b"), "/svc/Get", "request")
	require.NoError(t, err)
	require.NotEqual(t, a, b)
}
