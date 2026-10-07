// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
)

// convertCacheCompressionPreset converts config compression preset to interceptor preset.
func convertCacheCompressionPreset(p grpcconfig.CacheCompressionPreset) cache.CompressionPreset {
	switch p {
	case grpcconfig.CacheCompressionPresetNone:
		return cache.PresetNone
	case grpcconfig.CacheCompressionPresetFast:
		return cache.PresetFast
	case grpcconfig.CacheCompressionPresetBalanced:
		return cache.PresetBalanced
	case grpcconfig.CacheCompressionPresetBest:
		return cache.PresetBest
	default:
		return cache.PresetBalanced
	}
}
