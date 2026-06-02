// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"context"
	"strings"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// DefaultMetadataReadMaskHeader is the gRPC metadata key
// [MetadataReadExtractor] reads by default. It matches the Google Cloud
// convention for transporting an AIP-157 read mask through a side channel
// (HTTP query parameter "$fields" maps to this header on the wire).
const DefaultMetadataReadMaskHeader = "x-goog-fieldmask"

// metadataExtractorOptions holds configuration for [MetadataReadExtractor].
type metadataExtractorOptions struct {
	header string
}

// MetadataExtractorOption configures [MetadataReadExtractor].
type MetadataExtractorOption func(*metadataExtractorOptions)

// WithMetadataHeader overrides the gRPC metadata key the extractor reads.
// Empty name is ignored and the default [DefaultMetadataReadMaskHeader] is
// used. gRPC normalises metadata keys to lower-case on the wire, so the
// lookup is effectively case-insensitive regardless of the value passed.
func WithMetadataHeader(name string) MetadataExtractorOption {
	return func(o *metadataExtractorOptions) {
		if name == "" {
			return
		}
		o.header = name
	}
}

// MetadataReadExtractor builds a [ReadExtractorFunc] that reads the field
// mask from incoming gRPC metadata per AIP-157.
//
// The header value is a comma-separated list of paths in the JSON wire
// format of [google.protobuf.FieldMask]. Per AIP-157, an explicit "*" — or a
// missing header — means "all fields"; the extractor surfaces those as
// ok=false so the gRPC interceptor performs a silent passthrough without
// filtering. Surrounding whitespace around paths is trimmed; empty entries
// are dropped. When the metadata key carries multiple values, only the
// first is consulted.
//
// The default header name is [DefaultMetadataReadMaskHeader]; override it
// with [WithMetadataHeader].
func MetadataReadExtractor(opts ...MetadataExtractorOption) ReadExtractorFunc {
	cfg := metadataExtractorOptions{header: DefaultMetadataReadMaskHeader}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	header := cfg.header

	return func(ctx context.Context, _ proto.Message) (*fieldmaskpb.FieldMask, bool) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, false
		}

		values := md.Get(header)
		if len(values) == 0 {
			return nil, false
		}

		raw := strings.TrimSpace(values[0])
		if raw == "" || raw == "*" {
			return nil, false
		}

		paths := parseMetadataMaskHeader(raw)
		if len(paths) == 0 {
			return nil, false
		}

		return &fieldmaskpb.FieldMask{Paths: paths}, true
	}
}

// parseMetadataMaskHeader splits a comma-separated mask header value into
// its individual paths. Surrounding whitespace is trimmed and empty
// segments are dropped.
func parseMetadataMaskHeader(raw string) []string {
	parts := strings.Split(raw, ",")
	paths := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			paths = append(paths, trimmed)
		}
	}
	return paths
}
