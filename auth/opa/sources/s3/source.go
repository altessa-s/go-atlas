// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package s3

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/auth/opa"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// Source implements opa.PolicySource for S3-backed policies.
// It lists and downloads .rego files from an S3 bucket/prefix.
// Change detection is handled by the Manager.
type Source struct {
	opts       *options
	logger     *slog.Logger
	extensions map[string]struct{}
	// normalizedPrefix is opts.prefix guaranteed to end with "/" (or empty).
	// Pre-computed to avoid repeated normalization in relativeKey.
	normalizedPrefix string

	mu     sync.Mutex
	closed bool
}

// New creates a new S3 policy source.
func New(opts ...Option) (*Source, error) {
	o := newOptions(opts...)

	if o.s3Client == nil {
		return nil, fmt.Errorf("s3 client is required")
	}

	if o.bucket == "" {
		return nil, fmt.Errorf("bucket is required")
	}

	extensions := make(map[string]struct{}, len(o.extensions))
	for _, ext := range o.extensions {
		extensions[ext] = struct{}{}
	}

	var normalizedPrefix string
	if o.prefix != "" {
		normalizedPrefix = o.prefix
		if !strings.HasSuffix(normalizedPrefix, "/") {
			normalizedPrefix += "/"
		}
	}

	return &Source{
		opts:             o,
		logger:           cmp.Or(o.logger, slog.New(slog.DiscardHandler)),
		extensions:       extensions,
		normalizedPrefix: normalizedPrefix,
	}, nil
}

// Name returns the source identifier.
func (s *Source) Name() string {
	if s.opts.prefix != "" {
		return "s3:" + s.opts.bucket + "/" + s.opts.prefix
	}
	return "s3:" + s.opts.bucket
}

// Fetch lists and downloads all policy files from the configured S3 bucket/prefix.
func (s *Source) Fetch(ctx context.Context) (*opa.PolicyBundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, opa.ErrSourceClosed
	}

	keys, err := s.listObjects(ctx)
	if err != nil {
		return nil, err
	}

	modules := make(map[string][]byte)
	var data map[string]any

	for _, key := range keys {
		content, getErr := s.getObject(ctx, key)
		if getErr != nil {
			return nil, getErr
		}

		relKey := s.relativeKey(key)
		ext := path.Ext(key)

		if _, ok := s.extensions[ext]; ok {
			modules[relKey] = content
			continue
		}

		if s.opts.includeData && ext == ".json" {
			var jsonData any
			if unmarshalErr := json.Unmarshal(content, &jsonData); unmarshalErr != nil {
				return nil, coreerrs.Wrapf(unmarshalErr, "parse JSON data %s", key)
			}

			if data == nil {
				data = make(map[string]any)
			}

			dataKey := strings.TrimSuffix(relKey, ".json")
			data[dataKey] = jsonData
		}
	}

	if len(modules) == 0 {
		return nil, coreerrs.Wrapf(opa.ErrNoPolicyFiles, "s3://%s/%s", s.opts.bucket, s.opts.prefix)
	}

	var bundle *opa.PolicyBundle
	if data != nil {
		bundle = opa.NewPolicyBundleWithData(modules, data)
	} else {
		bundle = opa.NewPolicyBundle(modules)
	}

	s.logger.Debug("fetched policy bundle",
		slog.String("bucket", s.opts.bucket),
		slog.String("prefix", s.opts.prefix),
		slog.Int("modules", len(modules)),
		slog.String("revision", bundle.Revision))

	return bundle, nil
}

// Close releases resources held by the source.
func (s *Source) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	s.logger.Debug("s3 source closed",
		slog.String("bucket", s.opts.bucket),
		slog.String("prefix", s.opts.prefix))

	return nil
}

// Extensions returns the file extensions being loaded as a slice.
func (s *Source) Extensions() []string {
	return slices.Collect(maps.Keys(s.extensions))
}

// listObjects paginates through ListObjectsV2 and returns all matching object keys.
func (s *Source) listObjects(ctx context.Context) ([]string, error) {
	var keys []string

	input := &awss3.ListObjectsV2Input{
		Bucket: &s.opts.bucket,
	}

	if s.opts.prefix != "" {
		input.Prefix = &s.opts.prefix
	}

	for {
		output, err := s.opts.s3Client.ListObjectsV2(ctx, input)
		if err != nil {
			return nil, coreerrs.Wrapf(err, "list objects in bucket %s", s.opts.bucket)
		}

		for _, obj := range output.Contents {
			if obj.Key == nil {
				continue
			}

			key := *obj.Key
			ext := path.Ext(key)

			if _, ok := s.extensions[ext]; ok {
				keys = append(keys, key)
				continue
			}

			if s.opts.includeData && ext == ".json" {
				keys = append(keys, key)
			}
		}

		if output.IsTruncated == nil || !*output.IsTruncated {
			break
		}

		input.ContinuationToken = output.NextContinuationToken
	}

	return keys, nil
}

// getObject downloads a single object from S3 and returns its contents.
func (s *Source) getObject(ctx context.Context, key string) ([]byte, error) {
	output, err := s.opts.s3Client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: &s.opts.bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, coreerrs.Wrapf(err, "get object %s", key)
	}
	defer output.Body.Close()

	content, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, coreerrs.Wrapf(err, "read object body %s", key)
	}

	return content, nil
}

// relativeKey strips the prefix from an object key to produce a relative path.
func (s *Source) relativeKey(key string) string {
	if s.normalizedPrefix != "" && strings.HasPrefix(key, s.normalizedPrefix) {
		return key[len(s.normalizedPrefix):]
	}
	return key
}

// Compile-time interface check.
var _ opa.PolicySource = (*Source)(nil)
