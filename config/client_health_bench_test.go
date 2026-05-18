// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"testing"
	"time"
)

func BenchmarkHealthClientValidate(b *testing.B) {
	h := HealthClient{
		ServiceName: "benchmark-service",
	}

	for b.Loop() {
		_ = h.Validate()
	}
}

func BenchmarkHTTPHealthClientValidate(b *testing.B) {
	h := HTTPHealthClient{
		HealthClient: HealthClient{
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
	h := GRPCHealthClient{
		HealthClient: HealthClient{
			ServiceName: "benchmark-service",
		},
		StateMapper: HealthClientStateMapperDefault,
		PerTarget:   false,
	}

	for b.Loop() {
		_ = h.Validate()
	}
}

func BenchmarkHTTPHealthClientOptions(b *testing.B) {
	h := HTTPHealthClient{
		HealthClient: HealthClient{
			ServiceName: "benchmark-service",
		},
		RetryWindow:     60 * time.Second,
		RetryThreshold:  0.2,
		RetryMinSamples: 10,
		RetryBuckets:    60,
		PerHost:         true,
	}

	for b.Loop() {
		_ = h.Options()
	}
}

func BenchmarkGRPCHealthClientOptions(b *testing.B) {
	h := GRPCHealthClient{
		HealthClient: HealthClient{
			ServiceName: "benchmark-grpc-service",
		},
		StateMapper: HealthClientStateMapperStrict,
		PerTarget:   true,
	}

	for b.Loop() {
		_ = h.Options()
	}
}

func BenchmarkDefaultHealthClient(b *testing.B) {
	for b.Loop() {
		h := DefaultHealthClient()
		h.ServiceName = "benchmark-service" // Required field
		_ = h
	}
}

func BenchmarkDefaultHTTPHealthClient(b *testing.B) {
	for b.Loop() {
		h := DefaultHTTPHealthClient()
		h.ServiceName = "benchmark-service" // Required field
		_ = h
	}
}

func BenchmarkDefaultGRPCHealthClient(b *testing.B) {
	for b.Loop() {
		h := DefaultGRPCHealthClient()
		h.ServiceName = "benchmark-service" // Required field
		_ = h
	}
}
