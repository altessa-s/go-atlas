// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clienthealthconfig

import (
	"testing"
	"time"
)

func BenchmarkHealthClientValidate(b *testing.B) {
	h := Config{
		ServiceName: "benchmark-service",
	}

	for b.Loop() {
		_ = h.Validate()
	}
}

func BenchmarkHTTPHealthClientValidate(b *testing.B) {
	h := HTTP{
		Config: Config{
			ServiceName: "benchmark-service",
		},
		RetryWindow:     60 * time.Second,
		RetryThreshold:  0.2,
		RetryMinSamples: 10,
		RetryBuckets:    60,
		PerHost:         false,
	}

	for b.Loop() {
		_ = h.Validate()
	}
}

func BenchmarkGRPCHealthClientValidate(b *testing.B) {
	h := GRPC{
		Config: Config{
			ServiceName: "benchmark-service",
		},
		StateMapper: StateMapperDefault,
		PerTarget:   false,
	}

	for b.Loop() {
		_ = h.Validate()
	}
}

func BenchmarkDefaultHealthClient(b *testing.B) {
	for b.Loop() {
		h := Default()
		h.ServiceName = "benchmark-service" // Required field
		_ = h
	}
}

func BenchmarkDefaultHTTPHealthClient(b *testing.B) {
	for b.Loop() {
		h := DefaultHTTP()
		h.ServiceName = "benchmark-service" // Required field
		_ = h
	}
}

func BenchmarkDefaultGRPCHealthClient(b *testing.B) {
	for b.Loop() {
		h := DefaultGRPC()
		h.ServiceName = "benchmark-service" // Required field
		_ = h
	}
}
