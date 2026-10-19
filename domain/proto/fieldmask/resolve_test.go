// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func TestSelectsAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		paths   []string
		want    bool
		wantErr error
	}{
		{name: "empty", paths: nil},
		{name: "explicit paths", paths: []string{"name", "description"}},
		{name: "wildcard alone", paths: []string{"*"}, want: true},
		{name: "wildcard twice is the wildcard", paths: []string{"*", "*"}, want: true},
		{name: "wildcard with a path", paths: []string{"*", "name"}, wantErr: fieldmask.ErrWildcardNotAlone},
		{name: "wildcard with a nested path", paths: []string{"profile.secret", "*"}, wantErr: fieldmask.ErrWildcardNotAlone},
		{name: "nested wildcard is a path", paths: []string{"aliases.*"}},
		{name: "top-level wildcard branch is a path", paths: []string{"*.id"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := fieldmask.FromPaths(tc.paths...).SelectsAll()
			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestResolveSpellings(t *testing.T) {
	t.Parallel()

	md := (&pb.Resource{}).ProtoReflect().Descriptor()
	for path, want := range map[string]string{
		"tenant_id":                "tenant_id",
		"tenantId":                 "tenant_id",
		"server_meta":              "server_meta",
		"serverMeta":               "server_meta",
		"name":                     "name",
		"source_token":             "source_token",
		"sourceToken":              "source_token",
		"profile.displayName":      "profile.display_name",
		"aliases.*.displayName":    "aliases.*.display_name",
		"aliases.0.display_name":   "aliases.0.display_name",
		"labels.myKey.displayName": "labels.myKey.display_name",
		"serverMeta.someKey":       "server_meta.someKey",
		"labels.`a.b`.id":          "labels.`a.b`.id",
	} {
		got, err := fieldmask.FromPaths(path).Resolve(md)
		require.NoError(t, err, path)
		require.Equal(t, []string{want}, got.ToPaths(), path)
	}
}

// camelSchema returns a message whose fields are declared in lowerCamelCase,
// as some schemas do: protojson decodes the JSON mask path "noisePercent7d"
// to "noise_percent7d", which must still resolve to the declared name.
func camelSchema(t testing.TB) protoreflect.MessageDescriptor {
	t.Helper()
	fdp := &descriptorpb.FileDescriptorProto{
		Name:    new("camel_test.proto"),
		Package: new("fieldmasktest.camel"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Settings"),
			Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("noisePercent7d"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
					Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("noisePercent7d")},
				{Name: new("monthlyBudgetLimit"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("monthlyBudgetLimit")},
				{Name: new("baseURI"), Number: proto.Int32(3), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("baseURI")},
			},
		}},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	require.NoError(t, err)
	return fd.Messages().ByName("Settings")
}

func TestResolveSnakeCaseOfCamelDeclaredNames(t *testing.T) {
	t.Parallel()

	md := camelSchema(t)
	for path, want := range map[string]string{
		"noisePercent7d":       "noisePercent7d",
		"noise_percent7d":      "noisePercent7d",
		"monthly_budget_limit": "monthlyBudgetLimit",
		"base_u_r_i":           "baseURI",
		"baseURI":              "baseURI",
	} {
		got, err := fieldmask.FromPaths(path).Resolve(md)
		require.NoError(t, err, path)
		require.Equal(t, []string{want}, got.ToPaths(), path)
	}
	_, err := fieldmask.FromPaths("base_uri").Resolve(md)
	_, ok := errors.AsType[*fieldmask.ValidationError](err)
	require.True(t, ok, "base_uri is no spelling of baseURI: %v", err)
}

func TestResolveErrors(t *testing.T) {
	t.Parallel()

	md := (&pb.Resource{}).ProtoReflect().Descriptor()
	for _, path := range []string{"missing", "tenantID", "TenantId", "profile.missing", "name.sub", "tags.0.x"} {
		_, err := fieldmask.FromPaths(path).Resolve(md)
		verr, ok := errors.AsType[*fieldmask.ValidationError](err)
		require.True(t, ok, "%q: %v", path, err)
		require.Equal(t, path, verr.Path)
	}

	_, err := fieldmask.FromPaths("*", "name").Resolve(md)
	require.ErrorIs(t, err, fieldmask.ErrWildcardNotAlone)
}

func TestResolveKeepsWildcardAndEmpty(t *testing.T) {
	t.Parallel()

	md := (&pb.Resource{}).ProtoReflect().Descriptor()
	got, err := fieldmask.FromPaths("*").Resolve(md)
	require.NoError(t, err)
	require.Equal(t, []string{"*"}, got.ToPaths())

	got, err = fieldmask.FromPaths().Resolve(md)
	require.NoError(t, err)
	require.True(t, got.IsEmpty())
}

func TestResolveMergesAliases(t *testing.T) {
	t.Parallel()

	md := (&pb.Resource{}).ProtoReflect().Descriptor()
	got, err := fieldmask.FromPaths("tenantId", "tenant_id", "profile.displayName", "profile").Resolve(md)
	require.NoError(t, err)
	require.Equal(t, []string{"profile", "tenant_id"}, got.ToPaths())
}
