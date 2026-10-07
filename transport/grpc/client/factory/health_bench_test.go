// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/client/factory"

	clienthealthconfig "github.com/altessa-s/go-atlas/config/clienthealth"
)

func BenchmarkGRPCHealthClientOptions(b *testing.B) {
	h := clienthealthconfig.GRPC{
		Config: clienthealthconfig.Config{
			ServiceName: "benchmark-grpc-service",
		},
		StateMapper: clienthealthconfig.StateMapperStrict,
		PerTarget:   true,
	}

	for b.Loop() {
		_ = factory.HealthOptions(&h)
	}
}
