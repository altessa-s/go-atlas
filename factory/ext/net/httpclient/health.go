// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas/factory/ext/net/httpclient/observe"

	clienthealthconfig "github.com/altessa-s/go-atlas/config/clienthealth"
)

// HealthOptions returns the [observe] options for health monitoring of an
// HTTP client. Pass them to [observe.New] together with the health reporter.
//
// Returns nil if h is nil, allowing optional health configuration.
//
// Example:
//
//	client := httpclient.New(
//	    httpclient.WithObserver(observe.New(meter, coord, factory.HealthOptions(cfg.HTTPHealth)...)),
//	    httpclient.WithLogger(logger),
//	)
func HealthOptions(h *clienthealthconfig.HTTP) []observe.Option {
	if h == nil {
		return nil
	}

	var opts []observe.Option

	opts = slices.AppendIf(opts, h.ServiceName != "",
		observe.WithHealthServiceName(h.ServiceName))

	opts = slices.AppendIf(opts, h.RetryWindow > 0,
		observe.WithHealthRetryWindow(h.RetryWindow))

	opts = slices.AppendIf(opts, h.RetryThreshold > 0,
		observe.WithHealthRetryThreshold(h.RetryThreshold))

	opts = slices.AppendIf(opts, h.RetryMinSamples > 0,
		observe.WithHealthRetryMinSamples(h.RetryMinSamples))

	opts = slices.AppendIf(opts, h.RetryBuckets > 0,
		observe.WithHealthRetryBuckets(h.RetryBuckets))

	opts = slices.AppendIf(opts, h.PerHost,
		observe.WithPerHostHealthChecks())

	return opts
}
