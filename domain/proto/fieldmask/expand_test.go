// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	"google.golang.org/protobuf/proto"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
	testpb "github.com/altessa-s/go-atlas/proto/gen/fieldmasktest/v1"
)

func TestExpandMessagePaths(t *testing.T) {
	tests := []struct {
		name      string
		msg       *testpb.UpdateRequest
		paths     []string
		wantPaths []string
	}{
		{
			name:      "set leaf message expands to its subtree",
			msg:       &testpb.UpdateRequest{Options: &testpb.Options{Color: proto.String("red")}},
			paths:     []string{"options"},
			wantPaths: []string{"options.color", "options.size"},
		},
		{
			name:      "absent leaf message stays a leaf",
			msg:       &testpb.UpdateRequest{Id: "1"},
			paths:     []string{"options"},
			wantPaths: []string{"options"},
		},
		{
			name:      "protected sub-field is skipped",
			msg:       &testpb.UpdateRequest{ShippingAddress: &testpb.Address{Street: proto.String("Main")}},
			paths:     []string{"shipping_address"},
			wantPaths: []string{"shipping_address.street", "shipping_address.zip"},
		},
		{
			name:      "branch path keeps the caller's siblings",
			msg:       &testpb.UpdateRequest{Options: &testpb.Options{Color: proto.String("red")}},
			paths:     []string{"options.color"},
			wantPaths: []string{"options.color"},
		},
		{
			name:      "scalar, repeated, map and unknown paths are unchanged",
			msg:       &testpb.UpdateRequest{},
			paths:     []string{"status", "tags", "metadata", "nonexistent"},
			wantPaths: []string{"metadata", "nonexistent", "status", "tags"},
		},
		{
			name:      "non-message paths next to an expanded message are preserved",
			msg:       &testpb.UpdateRequest{Options: &testpb.Options{Color: proto.String("red")}},
			paths:     []string{"options", "id"},
			wantPaths: []string{"id", "options.color", "options.size"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fieldmask.FromPaths(tt.paths...).ExpandMessagePaths(tt.msg).ToPaths()
			slices.Sort(got)
			slices.Sort(tt.wantPaths)
			assert.Equal(t, tt.wantPaths, got)
		})
	}
}

func TestExpandMessagePaths_NilMessage(t *testing.T) {
	got := fieldmask.FromPaths("options").ExpandMessagePaths(nil).ToPaths()
	assert.Equal(t, []string{"options"}, got)
}

func TestExpandMessagePaths_EmptyMask(t *testing.T) {
	got := fieldmask.FieldMask{}.ExpandMessagePaths(&testpb.UpdateRequest{}).ToPaths()
	assert.Empty(t, got)
}

// Recurses into a set nested message and excludes every sub-field carrying an
// update-mask behavior at any depth.
func TestExpandMessagePaths_RecursesAndSkipsBehaviorFields(t *testing.T) {
	msg := &pb.NestedUpdateResourceRequest{
		Resource: &pb.Resource{Profile: &pb.Profile{DisplayName: "name"}},
	}

	got := fieldmask.FromPaths("resource").ExpandMessagePaths(msg).ToPaths()

	assert.Contains(t, got, "resource.description")
	assert.Contains(t, got, "resource.profile.display_name")
	assert.NotContains(t, got, "resource.name")        // REQUIRED
	assert.NotContains(t, got, "resource.id")          // IDENTIFIER
	assert.NotContains(t, got, "resource.tenant_id")   // IMMUTABLE
	assert.NotContains(t, got, "resource.create_time") // OUTPUT_ONLY
	assert.NotContains(t, got, "resource.audit")       // OUTPUT_ONLY message
	assert.NotContains(t, got, "resource.profile.updated_at")
}

// After expansion ApplyUpdateMask materializes the whole masked message: set
// sub-fields stay, unspecified ones get their default (so the sparse merge
// clears them).
func TestExpandMessagePaths_ThenApplyUpdateMask(t *testing.T) {
	msg := &testpb.UpdateRequest{
		Id:      "1",
		Options: &testpb.Options{Color: proto.String("blue")},
	}

	mask := fieldmask.FromPaths("options").ExpandMessagePaths(msg)
	require.NoError(t, mask.ApplyUpdateMask(msg))

	assert.Equal(t, "blue", msg.GetOptions().GetColor())
	sizeFD := msg.GetOptions().ProtoReflect().Descriptor().Fields().ByName("size")
	assert.True(t, msg.GetOptions().ProtoReflect().Has(sizeFD))
	assert.Equal(t, "", msg.GetId())
}
