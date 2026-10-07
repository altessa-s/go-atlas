// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"github.com/altessa-s/go-atlas/core/collections/slices"

	clienthealthconfig "github.com/altessa-s/go-atlas/config/clienthealth"
	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

// HealthOptions returns HTTP client options for health monitoring.
// The returned options should be used together with WithHealthCoordinator.
//
// Returns nil if h is nil, allowing optional health configuration.
//
// Example:
//
//	client := httpclient.New(append(
//	    []httpclient.Option{
//	        httpclient.WithHealthCoordinator(coord),
//	        httpclient.WithLogger(logger),
//	    },
//	    factory.HealthOptions(cfg.HTTPHealth)...,
//	)...)
func HealthOptions(h *clienthealthconfig.HTTP) []httpclient.Option {
	if h == nil {
		return nil
	}

	var opts []httpclient.Option

	opts = slices.AppendIf(opts, h.ServiceName != "",
		httpclient.WithHealthServiceName(h.ServiceName))

	opts = slices.AppendIf(opts, h.RetryWindow > 0,
		httpclient.WithHealthRetryWindow(h.RetryWindow))

	opts = slices.AppendIf(opts, h.RetryThreshold > 0,
		httpclient.WithHealthRetryThreshold(h.RetryThreshold))

	opts = slices.AppendIf(opts, h.RetryMinSamples > 0,
		httpclient.WithHealthRetryMinSamples(h.RetryMinSamples))

	opts = slices.AppendIf(opts, h.RetryBuckets > 0,
		httpclient.WithHealthRetryBuckets(h.RetryBuckets))

	opts = slices.AppendIf(opts, h.PerHost,
		httpclient.WithPerHostHealthChecks())

	return opts
}
