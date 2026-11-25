// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package s3_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/observability/health"

	corehash "github.com/altessa-s/go-atlas-core/encoding/hash"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	s3source "github.com/altessa-s/go-atlas/auth/opa/sources/s3"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestNew_RequiresReleaseManifest(t *testing.T) {
	t.Parallel()

	_, err := s3source.New(s3source.WithS3Client(&mockS3Client{}), s3source.WithBucket("test-bucket"))
	require.ErrorIs(t, err, opa.ErrReleaseManifestRequired)
}

func TestSource_Fetch_VerifiesChecksums(t *testing.T) {
	t.Parallel()

	const policy = "package main\ndefault allow = false"
	digest := corehash.SHA256HexBytes([]byte(policy))

	tests := []struct {
		name      string
		checksums map[string]string
		wantErr   error
	}{
		{name: "match", checksums: map[string]string{"main.rego": digest}},
		{name: "mismatch", checksums: map[string]string{"main.rego": strings.Repeat("0", 64)}, wantErr: s3source.ErrChecksumMismatch},
		{name: "unexpected file", checksums: map[string]string{}, wantErr: s3source.ErrUnexpectedPolicyFile},
		{name: "missing file", checksums: map[string]string{"main.rego": digest, "other.rego": digest}, wantErr: s3source.ErrMissingPolicyFile},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client := &mockS3Client{objects: map[string][]byte{"policies/main.rego": []byte(policy)}}
			source, err := s3source.New(
				s3source.WithS3Client(client),
				s3source.WithBucket("test-bucket"),
				s3source.WithPrefix("policies/"),
				s3source.WithChecksums(tc.checksums),
			)
			require.NoError(t, err)
			t.Cleanup(func() { _ = source.Close() })

			bundle, err := source.Fetch(t.Context())
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, policy, string(bundle.Modules["main.rego"]))
		})
	}
}

// Return a sibling even when the server was asked for the correct prefix.
type siblingListing struct {
	*mockS3Client
	sibling   string
	requested string
}

func (c *siblingListing) ListObjectsV2(ctx context.Context, in *awss3.ListObjectsV2Input, opts ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error) {
	if in.Prefix != nil {
		c.requested = *in.Prefix
	}
	out, err := c.mockS3Client.ListObjectsV2(ctx, in, opts...)
	if err == nil {
		out.Contents = append(out.Contents, types.Object{Key: &c.sibling})
	}
	return out, err
}

func TestDirectoryPrefix(t *testing.T) {
	t.Parallel()
	for _, prefix := range []string{"policies", "policies/"} {
		for _, manifest := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s-manifest-%t", prefix, manifest), func(t *testing.T) {
				t.Parallel()
				policy := []byte("package p")
				c := &siblingListing{
					mockS3Client: &mockS3Client{objects: map[string][]byte{"policies/main.rego": policy, "policies-archive/main.rego": policy}},
					sibling:      "policies-archive/main.rego",
				}
				opts := []s3source.Option{s3source.WithS3Client(c), s3source.WithBucket("b"), s3source.WithPrefix(prefix)}
				if manifest {
					opts = append(opts, writeRelease(t, c.mockS3Client, "policies/", false))
				} else {
					opts = append(opts, s3source.WithChecksums(map[string]string{"main.rego": corehash.SHA256HexBytes(policy)}))
				}
				src, err := s3source.New(opts...)
				require.NoError(t, err)
				b, err := src.Fetch(t.Context())
				require.Equal(t, "policies/", c.requested)
				require.NoError(t, err)
				require.Equal(t, map[string][]byte{"main.rego": policy}, b.Modules)
			})
		}
	}
}

type rollingClient struct {
	*mockS3Client
	afterRead func(string)
	requests  []awss3.GetObjectInput
}

func (c *rollingClient) GetObject(ctx context.Context, in *awss3.GetObjectInput, opts ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	c.requests = append(c.requests, *in)
	out, err := c.mockS3Client.GetObject(ctx, in, opts...)
	if c.afterRead != nil {
		c.afterRead(*in.Key)
	}
	return out, err
}

func TestS3Release_RolloutBetweenReads(t *testing.T) {
	t.Parallel()

	const prefix = "policies/"
	oldModule := []byte(`package policy
allow := data.z == "old"`)
	newModule := []byte(`package policy
allow := data.z != "new"`)
	oldData, newData := []byte(`"old"`), []byte(`"new"`)
	client := &rollingClient{mockS3Client: &mockS3Client{objects: map[string][]byte{
		prefix + "a.rego": oldModule,
		prefix + "z.json": oldData,
	}}}
	source, err := s3source.New(
		s3source.WithS3Client(client), s3source.WithBucket("bucket"), s3source.WithPrefix(prefix),
		s3source.WithIncludeData(), writeRelease(t, client.mockS3Client, prefix, true),
	)
	require.NoError(t, err)
	manager, err := opa.NewManager(t.Context(), source, "data.policy.allow")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })

	check := func(allow bool, status health.ServingStatus) {
		t.Helper()
		result, err := manager.Evaluator().Evaluate(t.Context(), nil)
		require.NoError(t, err)
		require.Equal(t, allow, result.Allow)
		require.Equal(t, status, manager.CheckHealth(t.Context()))
	}

	client.requests = nil
	client.afterRead = func(key string) {
		switch key {
		case prefix + "release.json":
			// The listing sees an upload newer than the selected manifest.
			client.objects[prefix+"z.json"] = []byte(`"staged"`)
		case prefix + "a.rego":
			client.objects[prefix+"a.rego"] = newModule
			client.objects[prefix+"z.json"] = newData
			writeRelease(t, client.mockS3Client, prefix, true)
		}
	}
	require.ErrorIs(t, manager.RunUpdateCycle(t.Context()), errPreconditionFailed)
	require.Len(t, client.requests, 3)
	require.Equal(t, prefix+"release.json", *client.requests[0].Key)
	require.Nil(t, client.requests[0].IfMatch)
	require.Equal(t, new(objectETag(oldModule)), client.requests[1].IfMatch)
	require.Equal(t, new(objectETag(oldData)), client.requests[2].IfMatch)
	check(true, health.StatusNotServing)

	client.afterRead = nil
	delete(client.objects, prefix+"z.json")
	require.ErrorIs(t, manager.RunUpdateCycle(t.Context()), s3source.ErrMissingPolicyFile)
	check(true, health.StatusNotServing)

	client.objects[prefix+"z.json"] = newData
	require.NoError(t, manager.RunUpdateCycle(t.Context()))
	check(false, health.StatusServing)
}

func TestS3Release_AtomicRootRollout(t *testing.T) {
	t.Parallel()

	const prefix, oldRoot, newRoot = "policies/", "releases/1", "releases/10"
	const manifestKey = prefix + "release.json"
	oldModule := []byte("package policy\nallow := data.z == \"old\"")
	newModule := []byte("package policy\nallow := data.z != \"new\"")
	oldData, newData := []byte(`"old"`), []byte(`"new"`)
	client := &rollingClient{mockS3Client: &mockS3Client{pageSize: 1, objects: map[string][]byte{
		prefix + oldRoot + "/a.rego": oldModule,
		prefix + oldRoot + "/z.json": oldData,
	}}}
	client.objects[manifestKey] = s3RootManifest(t, oldRoot, map[string][]byte{"a.rego": oldModule, "z.json": oldData})
	source, err := s3source.New(
		s3source.WithS3Client(client), s3source.WithBucket("bucket"), s3source.WithPrefix("policies"),
		s3source.WithIncludeData(), s3source.WithReleaseManifest("release.json"),
	)
	require.NoError(t, err)
	manager, err := opa.NewManager(t.Context(), source, "data.policy.allow")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	watch, err := manager.Watch(t.Context(), opa.DefaultWatchOptions())
	require.NoError(t, err)
	t.Cleanup(watch.Stop)
	oldRevision := manager.Revision()
	checkServing := func(revision string, allow bool) {
		t.Helper()
		require.Equal(t, health.StatusServing, manager.CheckHealth(t.Context()))
		require.Equal(t, revision, manager.Revision())
		result, err := manager.Evaluator().Evaluate(t.Context(), nil)
		require.NoError(t, err)
		require.Equal(t, allow, result.Allow)
	}

	client.objects[prefix+newRoot+"/a.rego"] = newModule
	require.NoError(t, manager.RunUpdateCycle(t.Context()))
	checkServing(oldRevision, true)
	client.objects[prefix+newRoot+"/z.json"] = newData
	client.requests = nil
	client.afterRead = func(key string) {
		if key == prefix+oldRoot+"/a.rego" {
			client.objects[manifestKey] = s3RootManifest(t, newRoot, map[string][]byte{"a.rego": newModule, "z.json": newData})
			checkServing(oldRevision, true)
		}
	}
	require.NoError(t, manager.RunUpdateCycle(t.Context()))
	require.Len(t, client.requests, 3)
	require.Equal(t, manifestKey, *client.requests[0].Key)
	require.Nil(t, client.requests[0].IfMatch)
	require.Equal(t, prefix+oldRoot+"/a.rego", *client.requests[1].Key)
	require.Equal(t, new(objectETag(oldModule)), client.requests[1].IfMatch)
	require.Equal(t, prefix+oldRoot+"/z.json", *client.requests[2].Key)
	require.Equal(t, new(objectETag(oldData)), client.requests[2].IfMatch)
	checkServing(oldRevision, true)
	require.Empty(t, watch.Events)

	require.NoError(t, manager.RunUpdateCycle(t.Context()))
	newRevision := manager.Revision()
	require.NotEqual(t, oldRevision, newRevision)
	checkServing(newRevision, false)
	require.NoError(t, manager.RunUpdateCycle(t.Context()))
	checkServing(newRevision, false)
	require.Len(t, watch.Events, 1)
	event := <-watch.Events
	require.Equal(t, opa.EventTypePolicyUpdated, event.Type)
	require.Equal(t, oldRevision, event.PreviousRevision)
	require.Equal(t, newRevision, event.Revision)
}

func TestS3Release_InvalidRootKeepsSnapshot(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		root    string
		wantErr error
	}{
		{name: "parent", root: "releases/../elsewhere", wantErr: coreerrs.ErrInvalidArgument},
		{name: "absolute", root: "/elsewhere", wantErr: coreerrs.ErrInvalidArgument},
		{name: "empty segment", root: "releases//1", wantErr: coreerrs.ErrInvalidArgument},
		{name: "empty root", wantErr: coreerrs.ErrInvalidArgument},
		{name: "missing", root: "releases/missing", wantErr: s3source.ErrMissingPolicyFile},
		{name: "incomplete", root: "releases/incomplete", wantErr: s3source.ErrMissingPolicyFile},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			const prefix = "policies/"
			policy := []byte("package policy\nallow := true")
			data := []byte(`"missing"`)
			client := &mockS3Client{objects: map[string][]byte{prefix + "a.rego": policy}}
			source, err := s3source.New(
				s3source.WithS3Client(client), s3source.WithBucket("bucket"), s3source.WithPrefix(prefix),
				s3source.WithIncludeData(), writeRelease(t, client, prefix, true),
			)
			require.NoError(t, err)
			manager, err := opa.NewManager(t.Context(), source, "data.policy.allow")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, manager.Close()) })
			revision := manager.Revision()

			client.objects[prefix+"releases/incomplete/a.rego"] = policy
			client.objects[prefix+"release.json"] = s3RootManifest(t, tc.root, map[string][]byte{"a.rego": policy, "z.json": data})
			require.ErrorIs(t, manager.RunUpdateCycle(t.Context()), tc.wantErr)
			require.Equal(t, health.StatusNotServing, manager.CheckHealth(t.Context()))
			require.Equal(t, revision, manager.Revision())
			result, err := manager.Evaluator().Evaluate(t.Context(), nil)
			require.NoError(t, err)
			require.True(t, result.Allow)
		})
	}
}

func TestS3Release_IgnoresSiblingInListing(t *testing.T) {
	t.Parallel()

	const prefix, root = "policies/", "releases/1"
	policy := []byte("package p")
	client := &siblingListing{
		mockS3Client: &mockS3Client{objects: map[string][]byte{
			prefix + root + "/main.rego": policy,
			prefix + "release.json":      s3RootManifest(t, root, map[string][]byte{"main.rego": policy}),
		}},
		sibling: prefix + "releases/10/main.rego",
	}
	source, err := s3source.New(s3source.WithS3Client(client), s3source.WithBucket("bucket"),
		s3source.WithPrefix(prefix), s3source.WithReleaseManifest("release.json"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, source.Close()) })

	bundle, err := source.Fetch(t.Context())
	require.NoError(t, err)
	require.Equal(t, prefix+root+"/", client.requested)
	require.Equal(t, map[string][]byte{"main.rego": policy}, bundle.Modules)
}

func s3RootManifest(t *testing.T, root string, files map[string][]byte) []byte {
	t.Helper()
	entries := make(map[string]map[string]string, len(files))
	for name, content := range files {
		entries[name] = map[string]string{"etag": objectETag(content), "sha256": corehash.SHA256HexBytes(content)}
	}
	payload, err := json.Marshal(map[string]any{"root": root, "files": entries})
	require.NoError(t, err)
	return payload
}
