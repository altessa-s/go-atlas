// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	"google.golang.org/grpc/metadata"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func TestMetadataReadExtractor(t *testing.T) {
	t.Parallel()

	req := &pb.GetResourceRequest{Name: "id-1"}

	t.Run("header present returns mask", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.MetadataReadExtractor()
		ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(
			fieldmask.DefaultMetadataReadMaskHeader, "name,authors.given_name",
		))

		mask, ok := extract(ctx, req)
		require.True(t, ok)
		require.Equal(t, []string{"name", "authors.given_name"}, mask.GetPaths())
	})

	t.Run("no metadata in ctx returns ok=false", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.MetadataReadExtractor()
		_, ok := extract(t.Context(), req)
		require.False(t, ok)
	})

	t.Run("header absent returns ok=false", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.MetadataReadExtractor()
		ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("other-header", "value"))
		_, ok := extract(ctx, req)
		require.False(t, ok)
	})

	t.Run("empty header value returns ok=false", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.MetadataReadExtractor()
		ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(
			fieldmask.DefaultMetadataReadMaskHeader, "",
		))
		_, ok := extract(ctx, req)
		require.False(t, ok)
	})

	t.Run("whitespace-only header returns ok=false", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.MetadataReadExtractor()
		ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(
			fieldmask.DefaultMetadataReadMaskHeader, "   ",
		))
		_, ok := extract(ctx, req)
		require.False(t, ok)
	})

	t.Run("star returns ok=false (AIP-157 all fields)", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.MetadataReadExtractor()
		ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(
			fieldmask.DefaultMetadataReadMaskHeader, "*",
		))
		_, ok := extract(ctx, req)
		require.False(t, ok)
	})

	t.Run("paths are trimmed and empties dropped", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.MetadataReadExtractor()
		ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(
			fieldmask.DefaultMetadataReadMaskHeader, "  name , , description  ,",
		))

		mask, ok := extract(ctx, req)
		require.True(t, ok)
		require.Equal(t, []string{"name", "description"}, mask.GetPaths())
	})

	t.Run("multiple values picks the first", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.MetadataReadExtractor()
		md := metadata.MD{}
		md.Append(fieldmask.DefaultMetadataReadMaskHeader, "name", "description")
		ctx := metadata.NewIncomingContext(t.Context(), md)

		mask, ok := extract(ctx, req)
		require.True(t, ok)
		require.Equal(t, []string{"name"}, mask.GetPaths())
	})

	t.Run("lookup is case-insensitive", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.MetadataReadExtractor()
		ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(
			"X-Goog-Fieldmask", "name",
		))

		mask, ok := extract(ctx, req)
		require.True(t, ok)
		require.Equal(t, []string{"name"}, mask.GetPaths())
	})

	t.Run("custom header name", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.MetadataReadExtractor(
			fieldmask.WithMetadataHeader("x-custom-fieldmask"),
		)
		ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(
			"x-custom-fieldmask", "name",
		))

		mask, ok := extract(ctx, req)
		require.True(t, ok)
		require.Equal(t, []string{"name"}, mask.GetPaths())
	})

	t.Run("empty option is ignored", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.MetadataReadExtractor(fieldmask.WithMetadataHeader(""))
		ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(
			fieldmask.DefaultMetadataReadMaskHeader, "name",
		))

		mask, ok := extract(ctx, req)
		require.True(t, ok)
		require.Equal(t, []string{"name"}, mask.GetPaths())
	})

	t.Run("nil option is skipped", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.MetadataReadExtractor(nil)
		ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(
			fieldmask.DefaultMetadataReadMaskHeader, "name",
		))

		mask, ok := extract(ctx, req)
		require.True(t, ok)
		require.Equal(t, []string{"name"}, mask.GetPaths())
	})
}
