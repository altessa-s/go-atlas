// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/fieldmask/update"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pbfieldmask "github.com/altessa-s/go-aip/fieldmask"
	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// repeat returns n copies of path.
func repeat(path string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = path
	}
	return out
}

// TestServerInterceptor_UpdateOptionsLimits pins that the mask limits of
// WithUpdateOptions apply to the update_mask as sent, duplicates included,
// both tighter and looser than the defaults, in plain and strict mode.
func TestServerInterceptor_UpdateOptionsLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		opts  []update.Option
		paths []string
		ok    bool
	}{
		{name: "tighter path count", opts: []update.Option{update.WithMaxPaths(1)}, paths: []string{"name", "description"}},
		{name: "tighter path count, duplicates", opts: []update.Option{update.WithMaxPaths(1)}, paths: []string{"description", "description"}},
		{name: "tighter path length", opts: []update.Option{update.WithMaxPathLength(5)}, paths: []string{"description"}},
		{name: "tighter path depth", opts: []update.Option{update.WithMaxPathDepth(1)}, paths: []string{"profile.display_name"}},
		{name: "default path count", paths: repeat("description", pbfieldmask.DefaultMaxPaths+1)},
		{name: "looser path count", opts: []update.Option{update.WithMaxPaths(2 * pbfieldmask.DefaultMaxPaths)}, paths: repeat("description", pbfieldmask.DefaultMaxPaths+1), ok: true},
		{name: "within limits", opts: []update.Option{update.WithMaxPaths(2)}, paths: []string{"name", "description"}, ok: true},
	}
	for _, tc := range tests {
		for _, strict := range []bool{false, true} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				opts := []fieldmask.Option{fieldmask.WithUpdateOptions(tc.opts...)}
				if strict {
					opts = append(opts, fieldmask.WithStrictMode())
				}
				req := &pb.UpdateResourceRequest{
					Resource:   &pb.Resource{Name: "n", Description: "d", Profile: &pb.Profile{DisplayName: "x"}},
					UpdateMask: &fieldmaskpb.FieldMask{Paths: tc.paths},
				}
				seen, _, err := runUnary(t, newUnary(t, opts...), "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
				if tc.ok {
					require.NoError(t, err)
					require.NotNil(t, seen)
					return
				}
				require.Equal(t, codes.InvalidArgument, status.Code(err), "strict=%v: %v", strict, err)
				require.Nil(t, seen)
			})
		}
	}
}

// TestServerInterceptor_UpdateOptionsPathValidation pins that the path
// validation of WithUpdateOptions governs the update_mask: Warn reports an
// unknown or malformed path and applies the rest, the default enforces.
func TestServerInterceptor_UpdateOptionsPathValidation(t *testing.T) {
	t.Parallel()

	for _, bad := range []string{"no_such", "name..x"} {
		var (
			mu       sync.Mutex
			reported []string
		)
		reporter := func(e *pbfieldmask.ValidationError) {
			mu.Lock()
			defer mu.Unlock()
			reported = append(reported, e.Path)
		}
		uni := newUnary(t, fieldmask.WithUpdateOptions(
			update.WithPathValidation(update.PathValidationWarn),
			update.WithPathValidationReporter(reporter),
		))
		req := &pb.UpdateResourceRequest{
			Resource:   &pb.Resource{Name: "n", Description: "d"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"description", bad}},
		}
		seen, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
		require.NoError(t, err, bad)
		require.NotNil(t, seen)
		require.Len(t, reported, 1, bad) // the bad path, as update spells it
		require.Equal(t, "d", seen.(*pb.UpdateResourceRequest).GetResource().GetDescription())

		req = &pb.UpdateResourceRequest{
			Resource:   &pb.Resource{Name: "n", Description: "d"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"description", bad}},
		}
		_, _, err = runUnary(t, newUnary(t), "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
		require.Equal(t, codes.InvalidArgument, status.Code(err), bad)
	}
}
