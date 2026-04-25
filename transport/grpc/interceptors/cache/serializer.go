// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"errors"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache/compression"

	"google.golang.org/protobuf/proto"
)

// Serializer handles serialization and deserialization of response data with method-specific compression support.
type Serializer interface {
	// MarshalForMethod serializes a response with method-specific compression settings
	MarshalForMethod(ctx context.Context, data any, methodConfig *MethodConfig) ([]byte, error)
	// UnmarshalForMethod deserializes bytes with method-specific decompression settings
	UnmarshalForMethod(ctx context.Context, data []byte, methodConfig *MethodConfig) (any, error)
}

// DefaultSerializer provides the default serialization behavior using protobuf with method-specific compression support.
type DefaultSerializer struct {
	compressor compression.Compressor
}

// NewDefaultSerializer creates a new DefaultSerializer with optional compression.
func NewDefaultSerializer(compressor compression.Compressor) *DefaultSerializer {
	if compressor == nil {
		compressor = compression.NewNoOpCompressor()
	}
	return &DefaultSerializer{
		compressor: compressor,
	}
}

// MarshalForMethod serializes a response with method-specific compression settings.
// If the method configuration specifies compression, it uses the method's Compressor.
// Otherwise, it falls back to the default serializer's Compressor.
func (s *DefaultSerializer) MarshalForMethod(ctx context.Context, data any, methodConfig *MethodConfig) ([]byte, error) {
	if msg, ok := data.(proto.Message); ok {
		data, err := proto.Marshal(msg)
		if err != nil {
			return nil, err
		}

		// Use method-specific Compressor if configured
		compressor := s.compressor
		if methodConfig.HasCompressor() {
			compressor = methodConfig.Compressor()
		}

		// Apply compression
		return compressor.Compress(ctx, data)
	}
	return nil, errors.New("response is not a proto.Message, cannot marshal to bytes")
}

// UnmarshalForMethod deserializes bytes with method-specific decompression settings.
// If the method configuration specifies compression, it uses the method's Compressor.
// Otherwise, it falls back to the default serializer's Compressor.
func (s *DefaultSerializer) UnmarshalForMethod(ctx context.Context, data []byte, methodConfig *MethodConfig) (any, error) {
	if msg, ok := methodConfig.ResponseProto.(proto.Message); ok {
		// Use method-specific Compressor if configured
		compressor := s.compressor
		if methodConfig.HasCompressor() {
			compressor = methodConfig.Compressor()
		}

		// Decompress data first
		decompressed, err := compressor.Decompress(ctx, data)
		if err != nil {
			return nil, err
		}

		resp := proto.Clone(msg)
		if err = proto.Unmarshal(decompressed, resp); err != nil {
			return nil, err
		}
		return resp, nil
	}
	return nil, errors.New("response is not a proto.Message, cannot unmarshal to bytes")
}
