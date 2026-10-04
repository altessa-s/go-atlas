// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/transport/grpc/client/factory"
)

func BenchmarkGRPCHealthClientOptions(b *testing.B) {
	h := config.GRPCHealthClient{
		HealthClient: config.HealthClient{
			ServiceName: "benchmark-grpc-service",
		},
		StateMapper: config.HealthClientStateMapperStrict,
		PerTarget:   true,
	}

	for b.Loop() {
		_ = factory.HealthOptions(&h)
	}
}
