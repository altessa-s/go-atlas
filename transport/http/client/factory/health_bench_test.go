// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/transport/http/client/factory"

	clienthealthconfig "github.com/altessa-s/go-atlas/config/clienthealth"
)

func BenchmarkHTTPHealthClientOptions(b *testing.B) {
	h := clienthealthconfig.HTTP{
		Config: clienthealthconfig.Config{
			ServiceName: "benchmark-service",
		},
		RetryWindow:     60 * time.Second,
		RetryThreshold:  0.2,
		RetryMinSamples: 10,
		RetryBuckets:    60,
		PerHost:         true,
	}

	for b.Loop() {
		_ = factory.HealthOptions(&h)
	}
}
