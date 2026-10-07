// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"slices"

	"github.com/cespare/xxhash/v2"

	"github.com/altessa-s/go-atlas/core/text/strings"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

const (
	maxMetadataKeys      = 100  // Maximum number of metadata keys to process (increased for legitimate use)
	maxMetadataKeySize   = 256  // Maximum size of a single metadata key in bytes (increased for longer keys)
	maxMetadataValueSize = 1024 // Maximum size of a single metadata value in bytes (increased for JWT tokens, etc.)
	maxMetadataValues    = 20   // Maximum number of values per metadata key (increased for multi-value headers)
)

// MetadataProcessor defines a function type for custom processing of gRPC metadata.
// It can be used to extract additional identifiers from headers (e.g., parsing JWT tokens from Authorization).
// The resulting map will be included in the cache key hash.
type MetadataProcessor func(ctx context.Context, md metadata.MD) map[string]string

// DefaultMetadataKeys is the standard set of gRPC metadata keys used for cache partitioning.
// These keys focus on user identity, organizational context, and versioning to prevent collisions.
var DefaultMetadataKeys = []string{
	"organization-id",
	"org-id",
	"user-id",
	"tenant-id",
	"x-user-id",
	"x-org-id",
	"x-organization-id",
	"x-tenant-id",
	"x-client-id",
	"x-api-version",
	"x-feature-flags",
}

// KeyGenerator generates cache keys from request data for cache storage and retrieval.
// The function receives the request context, gRPC method name, and request data,
// and returns a unique string key that identifies the request for caching purposes.
type KeyGenerator func(ctx context.Context, method string, req any) (string, error)

// DefaultKeyGenerator creates cache keys by combining the gRPC method name with a hash of the request data.
// It uses DefaultMetadataKeys to partition the cache by caller identity and context.
func DefaultKeyGenerator(ctx context.Context, method string, req any) (string, error) {
	return NewKeyGenerator(nil, nil)(ctx, method, req)
}

// newDefaultKeyGenerator builds the interceptor's default generator once, so
// the per-call path skips re-interning and sorting DefaultMetadataKeys.
func newDefaultKeyGenerator() KeyGenerator {
	return NewKeyGenerator(nil, nil)
}

// NewKeyGenerator creates a new KeyGenerator with custom metadata keys and an optional processor.
// If metadataKeys is empty, DefaultMetadataKeys will be used.
func NewKeyGenerator(metadataKeys []string, processor MetadataProcessor) KeyGenerator {
	if len(metadataKeys) == 0 {
		metadataKeys = DefaultMetadataKeys
	}

	// Intern keys to reduce memory usage as they're used frequently
	internedKeys := make([]string, len(metadataKeys))
	for i, k := range metadataKeys {
		internedKeys[i] = strings.InternLowerString(k)
	}
	slices.Sort(internedKeys)
	internedKeys = slices.Compact(internedKeys)

	return func(ctx context.Context, method string, req any) (string, error) {
		// Intern the method name to reduce memory usage
		internedMethod := strings.InternLowerString(method)

		// Create hash; every component is length-framed (see writeHashField).
		h := xxhash.New()
		writeHashField(h, tagMethod, internedMethod)

		if err := writeRequestToXXHasher(h, req); err != nil {
			return "", coreerrs.WrapOperation(err, "write request to hash")
		}

		// Add metadata if present
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			writeMetadataToXXHasher(h, md, internedKeys)

			// Run custom processor if provided
			if processor != nil {
				extra := processor(ctx, md)
				if len(extra) > 0 {
					writeProcessedMetadataToXXHasher(h, extra)
				}
			}
		}

		return formatHex16(h.Sum64()), nil
	}
}

// writeRequestToXXHasher serializes request data and writes it to the xxhash hasher.
func writeRequestToXXHasher(h *xxhash.Digest, req any) error {
	switch v := req.(type) {
	case proto.Message:
		if data, err := proto.Marshal(v); err == nil {
			writeHashBytes(h, tagRequest, data)
		} else {
			return coreerrs.WrapOperation(err, "marshal proto message")
		}
	default:
		// Try JSON marshaling for any type
		if data, err := json.Marshal(v); err == nil {
			writeHashBytes(h, tagRequest, data)
		} else {
			return coreerrs.Wrap(err, "unable to marshal request to JSON")
		}
	}
	return nil
}

// writeMetadataToXXHasher writes selected gRPC metadata to the xxhash hasher.
func writeMetadataToXXHasher(h *xxhash.Digest, md metadata.MD, relevantKeys []string) {
	processedKeys := 0
	for _, key := range relevantKeys {
		values, exists := md[key]
		if !exists || len(values) == 0 {
			continue
		}

		if len(key) > maxMetadataKeySize {
			continue
		}

		writeHashField(h, tagMetadataKey, key)

		processedValues := 0
		for _, v := range values {
			if len(v) > maxMetadataValueSize {
				continue
			}

			writeHashField(h, tagMetadataValue, strings.InternString(v))
			processedValues++

			if processedValues >= maxMetadataValues {
				break
			}
		}

		processedKeys++
		if processedKeys >= maxMetadataKeys {
			break
		}
	}
}

const hexDigits = "0123456789abcdef"

// formatHex16 formats a uint64 as a zero-padded 16-character hex string
// without using fmt.Sprintf, avoiding reflection and allocation overhead.
func formatHex16(v uint64) string {
	var buf [16]byte
	for i := 15; i >= 0; i-- {
		buf[i] = hexDigits[v&0x0f]
		v >>= 4
	}
	return string(buf[:])
}

// writeProcessedMetadataToXXHasher writes extra metadata from processor to the hasher.
func writeProcessedMetadataToXXHasher(h *xxhash.Digest, extra map[string]string) {
	// Sort keys for consistency
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	for _, k := range keys {
		v := extra[k]
		if len(k) > maxMetadataKeySize || len(v) > maxMetadataValueSize {
			continue
		}
		writeHashField(h, tagProcessedKey, strings.InternLowerString(k))
		writeHashField(h, tagProcessedValue, strings.InternString(v))
	}
}

// Field tags written by [writeHashField]; each kind of key component has its
// own tag so a value can never be read back as a key, method or request.
const (
	tagMethod byte = iota + 1
	tagRequest
	tagMetadataKey
	tagMetadataValue
	tagProcessedKey
	tagProcessedValue
)

// writeHashField writes one key component as tag, 8-byte little-endian
// length, payload. The framing makes the hashed stream uniquely decodable, so
// distinct inputs cannot collide by concatenation (e.g. metadata
// tenant-id=["auser-idb"] versus tenant-id=["a"], user-id=["b"]).
func writeHashField(h *xxhash.Digest, tag byte, payload string) {
	writeHashHeader(h, tag, len(payload))
	h.WriteString(payload) //nolint:errcheck // xxhash.Digest.WriteString never returns an error
}

// writeHashBytes is the []byte counterpart of [writeHashField].
func writeHashBytes(h *xxhash.Digest, tag byte, payload []byte) {
	writeHashHeader(h, tag, len(payload))
	h.Write(payload) //nolint:errcheck // xxhash.Digest.Write never returns an error
}

func writeHashHeader(h *xxhash.Digest, tag byte, n int) {
	var hdr [9]byte
	hdr[0] = tag
	binary.LittleEndian.PutUint64(hdr[1:], uint64(n))
	h.Write(hdr[:]) //nolint:errcheck // xxhash.Digest.Write never returns an error
}
