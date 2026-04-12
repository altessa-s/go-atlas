// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package s3_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa"

	s3source "github.com/altessa-s/go-atlas/auth/opa/sources/s3"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// mockS3Client implements s3source.S3API for testing.
type mockS3Client struct {
	objects map[string][]byte // key → content
	// pages splits listing into pages of this size; 0 means single page.
	pageSize int
}

func (m *mockS3Client) ListObjectsV2(_ context.Context, params *awss3.ListObjectsV2Input, _ ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error) {
	var allKeys []string
	prefix := ""
	if params.Prefix != nil {
		prefix = *params.Prefix
	}

	for k := range m.objects {
		if strings.HasPrefix(k, prefix) {
			allKeys = append(allKeys, k)
		}
	}

	// Sort for deterministic pagination.
	slices.Sort(allKeys)

	// Determine pagination window.
	startIdx := 0
	if params.ContinuationToken != nil {
		for i, k := range allKeys {
			if k == *params.ContinuationToken {
				startIdx = i
				break
			}
		}
	}

	pageSize := len(allKeys) // default: all in one page
	if m.pageSize > 0 {
		pageSize = m.pageSize
	}

	endIdx := min(startIdx+pageSize, len(allKeys))

	page := allKeys[startIdx:endIdx]

	var contents []types.Object
	for _, k := range page {
		contents = append(contents, types.Object{Key: &k})
	}

	isTruncated := endIdx < len(allKeys)
	out := &awss3.ListObjectsV2Output{
		Contents:    contents,
		IsTruncated: &isTruncated,
	}

	if isTruncated {
		next := allKeys[endIdx]
		out.NextContinuationToken = &next
	}

	return out, nil
}

func (m *mockS3Client) GetObject(_ context.Context, params *awss3.GetObjectInput, _ ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	key := ""
	if params.Key != nil {
		key = *params.Key
	}

	content, ok := m.objects[key]
	if !ok {
		return nil, errors.New("NoSuchKey: The specified key does not exist")
	}

	return &awss3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader(content)),
	}, nil
}

func TestNew_MissingClient(t *testing.T) {
	t.Parallel()

	_, err := s3source.New(s3source.WithBucket("bucket"))
	require.Error(t, err, "New() without s3 client should fail")
}

func TestNew_MissingBucket(t *testing.T) {
	t.Parallel()

	_, err := s3source.New(s3source.WithS3Client(&mockS3Client{}))
	require.Error(t, err, "New() without bucket should fail")
}

func TestSource_Name(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prefix   string
		expected string
	}{
		{"no prefix", "", "s3:my-bucket"},
		{"with prefix", "policies/", "s3:my-bucket/policies/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := []s3source.Option{
				s3source.WithS3Client(&mockS3Client{objects: map[string][]byte{}}),
				s3source.WithBucket("my-bucket"),
			}
			if tt.prefix != "" {
				opts = append(opts, s3source.WithPrefix(tt.prefix))
			}

			source, err := s3source.New(opts...)
			require.NoError(t, err)
			defer source.Close()

			require.Equal(t, tt.expected, source.Name())
		})
	}
}

func TestSource_Fetch_SingleFile(t *testing.T) {
	t.Parallel()

	client := &mockS3Client{
		objects: map[string][]byte{
			"policies/main.rego": []byte("package main\ndefault allow = false"),
		},
	}

	source, err := s3source.New(
		s3source.WithS3Client(client),
		s3source.WithBucket("test-bucket"),
		s3source.WithPrefix("policies/"),
	)
	require.NoError(t, err)
	defer source.Close()

	bundle, err := source.Fetch(t.Context())
	require.NoError(t, err)
	require.Len(t, bundle.Modules, 1)

	_, ok := bundle.Modules["main.rego"]
	require.True(t, ok, "bundle.Modules keys = %v, missing %q", moduleKeys(bundle), "main.rego")
	require.NotEmpty(t, bundle.Revision)
}

func TestSource_Fetch_MultipleFiles(t *testing.T) {
	t.Parallel()

	client := &mockS3Client{
		objects: map[string][]byte{
			"policies/auth/main.rego":    []byte("package auth\ndefault allow = false"),
			"policies/auth/helpers.rego": []byte("package auth\nhelper = true"),
			"policies/rbac/roles.rego":   []byte("package rbac\nrole = \"admin\""),
		},
	}

	source, err := s3source.New(
		s3source.WithS3Client(client),
		s3source.WithBucket("test-bucket"),
		s3source.WithPrefix("policies/"),
	)
	require.NoError(t, err)
	defer source.Close()

	bundle, err := source.Fetch(t.Context())
	require.NoError(t, err)
	require.Len(t, bundle.Modules, 3)

	for _, key := range []string{"auth/main.rego", "auth/helpers.rego", "rbac/roles.rego"} {
		_, ok := bundle.Modules[key]
		require.True(t, ok, "bundle.Modules keys = %v, missing %q", moduleKeys(bundle), key)
	}
}

func TestSource_Fetch_Empty(t *testing.T) {
	t.Parallel()

	client := &mockS3Client{
		objects: map[string][]byte{
			"policies/readme.txt": []byte("no policies here"),
		},
	}

	source, err := s3source.New(
		s3source.WithS3Client(client),
		s3source.WithBucket("test-bucket"),
		s3source.WithPrefix("policies/"),
	)
	require.NoError(t, err)
	defer source.Close()

	_, err = source.Fetch(t.Context())
	require.Error(t, err)
	require.True(t, errors.Is(err, opa.ErrNoPolicyFiles), "Fetch() error = %v, want ErrNoPolicyFiles", err)
}

func TestSource_Fetch_Closed(t *testing.T) {
	t.Parallel()

	client := &mockS3Client{
		objects: map[string][]byte{
			"policies/test.rego": []byte("package test\n"),
		},
	}

	source, err := s3source.New(
		s3source.WithS3Client(client),
		s3source.WithBucket("test-bucket"),
		s3source.WithPrefix("policies/"),
	)
	require.NoError(t, err)

	require.NoError(t, source.Close())

	_, err = source.Fetch(t.Context())
	require.Error(t, err)
	require.True(t, errors.Is(err, opa.ErrSourceClosed), "Fetch() error = %v, want ErrSourceClosed", err)
}

func TestSource_Fetch_WithData(t *testing.T) {
	t.Parallel()

	client := &mockS3Client{
		objects: map[string][]byte{
			"policies/test.rego": []byte("package test\ndefault allow = false"),
			"policies/data.json": []byte(`{"users": ["alice", "bob"]}`),
		},
	}

	source, err := s3source.New(
		s3source.WithS3Client(client),
		s3source.WithBucket("test-bucket"),
		s3source.WithPrefix("policies/"),
		s3source.WithIncludeData(),
	)
	require.NoError(t, err)
	defer source.Close()

	bundle, err := source.Fetch(t.Context())
	require.NoError(t, err)
	require.Len(t, bundle.Modules, 1)
	require.NotNil(t, bundle.Data)

	_, ok := bundle.Data["data"]
	require.True(t, ok, "bundle.Data keys = %v, missing %q", dataKeys(bundle), "data")
}

func TestSource_Fetch_WithDataDisabled(t *testing.T) {
	t.Parallel()

	client := &mockS3Client{
		objects: map[string][]byte{
			"policies/test.rego": []byte("package test\ndefault allow = false"),
			"policies/data.json": []byte(`{"users": ["alice"]}`),
		},
	}

	source, err := s3source.New(
		s3source.WithS3Client(client),
		s3source.WithBucket("test-bucket"),
		s3source.WithPrefix("policies/"),
		// includeData not set — .json files should be ignored
	)
	require.NoError(t, err)
	defer source.Close()

	bundle, err := source.Fetch(t.Context())
	require.NoError(t, err)
	require.Nil(t, bundle.Data, "bundle.Data should be nil when includeData is false")
}

func TestSource_Fetch_Pagination(t *testing.T) {
	t.Parallel()

	client := &mockS3Client{
		objects: map[string][]byte{
			"p/a.rego": []byte("package a"),
			"p/b.rego": []byte("package b"),
			"p/c.rego": []byte("package c"),
			"p/d.rego": []byte("package d"),
			"p/e.rego": []byte("package e"),
		},
		pageSize: 2, // force 3 pages: [a,b], [c,d], [e]
	}

	source, err := s3source.New(
		s3source.WithS3Client(client),
		s3source.WithBucket("test-bucket"),
		s3source.WithPrefix("p/"),
	)
	require.NoError(t, err)
	defer source.Close()

	bundle, err := source.Fetch(t.Context())
	require.NoError(t, err)
	require.Len(t, bundle.Modules, 5)
}

func TestSource_Fetch_NoPrefix(t *testing.T) {
	t.Parallel()

	client := &mockS3Client{
		objects: map[string][]byte{
			"main.rego": []byte("package main\ndefault allow = false"),
		},
	}

	source, err := s3source.New(
		s3source.WithS3Client(client),
		s3source.WithBucket("test-bucket"),
	)
	require.NoError(t, err)
	defer source.Close()

	bundle, err := source.Fetch(t.Context())
	require.NoError(t, err)
	require.Len(t, bundle.Modules, 1)

	_, ok := bundle.Modules["main.rego"]
	require.True(t, ok, "bundle.Modules keys = %v, missing %q", moduleKeys(bundle), "main.rego")
}

func TestSource_Close_Idempotent(t *testing.T) {
	t.Parallel()

	source, err := s3source.New(
		s3source.WithS3Client(&mockS3Client{objects: map[string][]byte{}}),
		s3source.WithBucket("test-bucket"),
	)
	require.NoError(t, err)

	require.NoError(t, source.Close(), "first Close() failed")
	require.NoError(t, source.Close(), "second Close() failed")
}

// moduleKeys returns the sorted module key names from a bundle for diagnostic output.
func moduleKeys(b *opa.PolicyBundle) []string {
	keys := make([]string, 0, len(b.Modules))
	for k := range b.Modules {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// dataKeys returns the sorted data key names from a bundle for diagnostic output.
func dataKeys(b *opa.PolicyBundle) []string {
	keys := make([]string, 0, len(b.Data))
	for k := range b.Data {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
