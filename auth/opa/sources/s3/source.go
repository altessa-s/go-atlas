// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package s3

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/auth/opa/internal/checksum"
	"github.com/altessa-s/go-atlas/auth/opa/internal/jsondoc"

	coremaps "github.com/altessa-s/go-atlas-core/collections/maps"
	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// maxObjectBytes caps how much of a policy object is read into memory.
// Rego policies and data documents are typically well under this; the cap
// guards against a misconfigured bucket/prefix pointing at a huge object.
const maxObjectBytes = 32 << 20 // 32 MiB

// ErrInsecureEndpoint is returned by the OPA factory when the S3 endpoint it
// builds a client for is not https:// and plain http was not explicitly
// allowed. SigV4 protects the credentials but not the policy content, which a
// MITM could swap. The source itself takes an injected client whose endpoint it
// cannot see, so the check lives where the client is built.
var ErrInsecureEndpoint = errors.New("s3 endpoint must use https")

// Release manifest sentinel errors.
var (
	// ErrChecksumMismatch is returned when an object's SHA-256 digest does not
	// match its manifest entry.
	ErrChecksumMismatch = errors.New("s3 policy object checksum mismatch")
	// ErrUnexpectedPolicyFile is returned when a fetched object has no
	// manifest entry.
	ErrUnexpectedPolicyFile = errors.New("s3 policy object not in checksums manifest")
	// ErrMissingPolicyFile is returned when a manifest entry has no fetched
	// object.
	ErrMissingPolicyFile = errors.New("s3 policy object in checksums manifest not found")
)

// Source implements opa.PolicySource for S3-backed policies.
// It lists and downloads .rego files from an S3 bucket/prefix.
// Change detection is handled by the Manager.
type Source struct {
	opts       *options
	logger     *slog.Logger
	extensions *coremaps.ImmutableMap[string, struct{}]
	checksums  *checksum.Manifest
	// normalizedPrefix is opts.prefix guaranteed to end with "/" (or empty).
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
	if o.releaseManifest == "" && o.checksums == nil {
		return nil, opa.ErrReleaseManifestRequired
	}
	if o.releaseManifest != "" && (!checksum.ValidPath(o.releaseManifest) || o.checksums != nil) {
		return nil, errors.New("invalid or conflicting s3 release manifest configuration")
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

	manifest := checksum.New(o.checksums)
	o.checksums = nil

	return &Source{
		opts:             o,
		logger:           cmp.Or(o.logger, slog.New(slog.DiscardHandler)),
		extensions:       coremaps.NewImmutableMap(extensions),
		checksums:        manifest,
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

	prefix, manifest, etags, err := s.readManifest(ctx)
	if err != nil {
		return nil, err
	}

	objects, err := s.listObjects(ctx, prefix)
	if err != nil {
		return nil, err
	}

	modules := make(map[string][]byte)
	var data map[string]any
	loaded := make(map[string]string, len(objects))

	for _, object := range objects {
		key := *object.Key
		relKey := strings.TrimPrefix(key, prefix)
		ext := path.Ext(key)

		etag := object.ETag
		if etags != nil {
			pin, ok := etags[relKey]
			if !ok {
				return nil, coreerrs.Wrapf(ErrUnexpectedPolicyFile, "%s", relKey)
			}
			etag = &pin
		}
		if etag == nil || !validETag(*etag) {
			return nil, fmt.Errorf("missing or invalid ETag for %q", key)
		}
		content, err := s.getObject(ctx, key, etag)
		if err != nil {
			return nil, err
		}
		if err := manifest.Verify(relKey, content, ErrChecksumMismatch, ErrUnexpectedPolicyFile); err != nil {
			return nil, err
		}
		loaded[relKey] = key

		if s.extensions.Contains(ext) {
			modules[relKey] = content
			continue
		}

		if s.opts.includeData && ext == ".json" {
			jsonData, unmarshalErr := jsondoc.Decode(content)
			if unmarshalErr != nil {
				return nil, coreerrs.Wrapf(unmarshalErr, "parse JSON data %s", key)
			}

			if data == nil {
				data = make(map[string]any)
			}

			dataKey := strings.TrimSuffix(relKey, ".json")
			data[dataKey] = jsonData
		}
	}

	if err := manifest.Covered(loaded, ErrMissingPolicyFile); err != nil {
		return nil, err
	}

	if len(modules) == 0 {
		return nil, coreerrs.Wrapf(opa.ErrNoPolicyFiles, "s3://%s/%s", s.opts.bucket, s.opts.prefix)
	}

	bundle, revisionErr := opa.NewPolicyBundleWithData(modules, data)
	if revisionErr != nil {
		return nil, revisionErr
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
	return slices.Collect(s.extensions.Keys())
}

// listObjects paginates through ListObjectsV2 and returns all matching object keys.
func (s *Source) listObjects(ctx context.Context, prefix string) ([]types.Object, error) {
	var objects []types.Object

	input := &awss3.ListObjectsV2Input{
		Bucket: &s.opts.bucket,
	}

	if prefix != "" {
		input.Prefix = &prefix
	}
	manifestKey := s.normalizedPrefix + s.opts.releaseManifest

	for {
		output, err := s.opts.s3Client.ListObjectsV2(ctx, input)
		if err != nil {
			return nil, coreerrs.Wrapf(err, "list objects in bucket %s", s.opts.bucket)
		}

		for _, obj := range output.Contents {
			if obj.Key == nil || !strings.HasPrefix(*obj.Key, prefix) {
				continue
			}

			key := *obj.Key
			if key == manifestKey {
				continue
			}
			ext := path.Ext(key)
			objects = coreslices.AppendIf(objects,
				s.extensions.Contains(ext) || (s.opts.includeData && ext == ".json"), obj)
		}

		if output.IsTruncated == nil || !*output.IsTruncated {
			break
		}

		input.ContinuationToken = output.NextContinuationToken
	}

	return objects, nil
}

// getObject downloads a single object from S3 and returns its contents.
func (s *Source) getObject(ctx context.Context, key string, etag *string) ([]byte, error) {
	output, err := s.opts.s3Client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket:  &s.opts.bucket,
		Key:     &key,
		IfMatch: etag,
	})
	if err != nil {
		return nil, coreerrs.Wrapf(err, "get object %s", key)
	}
	defer func() { _ = output.Body.Close() }()

	content, err := io.ReadAll(io.LimitReader(output.Body, maxObjectBytes+1))
	if err != nil {
		return nil, coreerrs.Wrapf(err, "read object body %s", key)
	}
	if len(content) > maxObjectBytes {
		return nil, fmt.Errorf("object %s exceeds %d bytes", key, maxObjectBytes)
	}

	return content, nil
}

type releaseEntry struct {
	ETag   string `json:"etag"`
	SHA256 string `json:"sha256"`
}

func (s *Source) readManifest(ctx context.Context) (string, *checksum.Manifest, map[string]string, error) {
	if s.opts.releaseManifest == "" {
		return s.normalizedPrefix, s.checksums, nil, nil
	}

	content, err := s.getObject(ctx, s.normalizedPrefix+s.opts.releaseManifest, nil)
	if err != nil {
		return "", nil, nil, err
	}
	root, entries, err := checksum.DecodeRelease[releaseEntry](content)
	if err != nil {
		return "", nil, nil, coreerrs.WrapOperation(err, "decode s3 release manifest")
	}
	digests := make(map[string]string, len(entries))
	etags := make(map[string]string, len(entries))
	for name, entry := range entries {
		if !checksum.ValidDigest(entry.SHA256) || !validETag(entry.ETag) {
			return "", nil, nil, fmt.Errorf("invalid SHA-256 digest or ETag for %q", name)
		}
		digests[name] = entry.SHA256
		etags[name] = entry.ETag
	}
	prefix := s.normalizedPrefix
	if root != "" {
		prefix += root + "/"
	}
	return prefix, checksum.New(digests), etags, nil
}

// S3 returns quoted entity tags. Accept exactly one opaque tag, excluding
// wildcard and list forms of If-Match.
func validETag(tag string) bool {
	value, quoted := strings.CutPrefix(tag, `"`)
	value, closed := strings.CutSuffix(value, `"`)
	if !quoted || !closed || value == "" {
		return false
	}
	for _, b := range []byte(value) {
		if b <= ' ' || b == '"' || b == 127 {
			return false
		}
	}
	return true
}

// Compile-time interface check.
var _ opa.PolicySource = (*Source)(nil)
