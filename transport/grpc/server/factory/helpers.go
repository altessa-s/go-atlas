// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"
)

// convertCacheCompressionPreset converts config compression preset to interceptor preset.
func convertCacheCompressionPreset(p config.GrpcInterCacheCompressionPreset) cache.CompressionPreset {
	switch p {
	case config.GrpcInterCacheCompressionPresetNone:
		return cache.PresetNone
	case config.GrpcInterCacheCompressionPresetFast:
		return cache.PresetFast
	case config.GrpcInterCacheCompressionPresetBalanced:
		return cache.PresetBalanced
	case config.GrpcInterCacheCompressionPresetBest:
		return cache.PresetBest
	default:
		return cache.PresetBalanced
	}
}
