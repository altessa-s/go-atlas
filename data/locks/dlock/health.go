// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlock

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/data/locks/dlock/providers"
	"github.com/altessa-s/go-atlas/observability/health"
)

var _ health.Checker = (*DLock)(nil)

// CheckHealth implements [health.Checker]. It returns
// [health.StatusNotServing] when the provider is nil or its [Probe]
// reports an error. Providers that don't implement [providers.Prober]
// are reported as Serving, since DLock has no way to probe them.
//
// Register a DLock with a coordinator via [WithHealthCoordinator] and,
// optionally, [WithHealthServiceName] to override the default
// "dlock" service name.
func (l *DLock) CheckHealth(ctx context.Context) health.ServingStatus {
	if l.provider == nil {
		return health.StatusNotServing
	}
	prober, ok := l.provider.(providers.Prober)
	if !ok {
		return health.StatusServing
	}
	if err := prober.Probe(ctx); err != nil {
		l.logger.DebugContext(ctx, "dlock provider probe failed", slog.Any("error", err))
		return health.StatusNotServing
	}
	return health.StatusServing
}
