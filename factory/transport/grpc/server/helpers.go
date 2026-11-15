// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	compression "github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache/compression"
)

// convertCacheCompressionPreset converts config compression preset to interceptor preset.
func convertCacheCompressionPreset(p grpcconfig.CacheCompressionPreset) compression.Preset {
	switch p {
	case grpcconfig.CacheCompressionPresetNone:
		return compression.PresetNone
	case grpcconfig.CacheCompressionPresetFast:
		return compression.PresetFast
	case grpcconfig.CacheCompressionPresetBalanced:
		return compression.PresetBalanced
	case grpcconfig.CacheCompressionPresetBest:
		return compression.PresetBest
	default:
		return compression.PresetBalanced
	}
}
