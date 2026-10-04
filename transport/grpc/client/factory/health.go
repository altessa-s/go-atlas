// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"

	grpcclient "github.com/altessa-s/go-atlas/transport/grpc/client"
)

// HealthOptions returns gRPC client options for health monitoring.
// The returned options should be used together with WithHealthCoordinator.
//
// Returns nil if h is nil, allowing optional health configuration.
//
// Example:
//
//	client, err := grpcclient.New(ctx, target, append(
//	    []grpcclient.Option{
//	        grpcclient.WithHealthCoordinator(coord),
//	        grpcclient.WithLogger(logger),
//	    },
//	    factory.HealthOptions(cfg.GRPCHealth)...,
//	)...)
func HealthOptions(h *config.GRPCHealthClient) []grpcclient.Option {
	if h == nil {
		return nil
	}

	var opts []grpcclient.Option

	opts = slices.AppendIf(opts, h.ServiceName != "",
		grpcclient.WithHealthServiceName(h.ServiceName))

	// Note: StateMapper requires a function, not a string.
	// The actual mapper implementation would need to be provided
	// by the caller or through a factory method.
	// For now, we only set the service name.

	return opts
}
