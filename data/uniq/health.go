// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uniq

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/data/uniq/providers"
	"github.com/altessa-s/go-atlas/observability/health"
)

var _ health.Checker = (*Uniq)(nil)

// CheckHealth implements [health.Checker]. It returns
// [health.StatusNotServing] when the provider is nil or its [Probe]
// reports an error. Providers that don't implement [providers.Prober]
// are reported as Serving, since Uniq has no way to probe them.
//
// Register a Uniq with a coordinator via [WithHealthCoordinator] and,
// optionally, [WithHealthServiceName] to override the default
// "uniq" service name.
func (s *Uniq) CheckHealth(ctx context.Context) health.ServingStatus {
	if s.provider == nil {
		return health.StatusNotServing
	}
	prober, ok := s.provider.(providers.Prober)
	if !ok {
		return health.StatusServing
	}
	if err := prober.Probe(ctx); err != nil {
		s.logger.DebugContext(ctx, "uniq provider probe failed", slog.Any("error", err))
		return health.StatusNotServing
	}
	return health.StatusServing
}
