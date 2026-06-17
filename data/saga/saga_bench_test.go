// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga_test

import (
	"context"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/saga"

	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
)

// noopStore is an allocation-light [saga.Store] used by benchmarks to isolate
// orchestrator cost from real persistence.
type noopStore struct{}

func (noopStore) Create(context.Context, *saga.Instance) error { return nil }
func (noopStore) Get(context.Context, string) (*saga.Instance, error) {
	return nil, sagaerrs.ErrInstanceNotFound
}
func (noopStore) Update(context.Context, *saga.Instance) error { return nil }
func (noopStore) FetchRecoverable(context.Context, time.Time, int) ([]*saga.Instance, error) {
	return nil, nil
}
func (noopStore) Delete(context.Context, string) error { return nil }

func noop(_ context.Context, _ *order) error { return nil }

func BenchmarkStart(b *testing.B) {
	def := saga.NewDefinition[order]("bench").
		Step("a", noop).Compensate(noop).
		Step("b", noop).Compensate(noop).
		Step("c", noop).
		MustBuild()
	orch := saga.New(noopStore{}, def)
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		_, _ = orch.Start(ctx, "bench", order{})
	}
}

func BenchmarkStartParallel(b *testing.B) {
	def := saga.NewDefinition[order]("bench-parallel").
		Parallel("fanout",
			saga.NewStep("a", noop),
			saga.NewStep("b", noop),
			saga.NewStep("c", noop),
			saga.NewStep("d", noop),
		).
		MustBuild()
	orch := saga.New(noopStore{}, def, saga.WithStepConcurrency(4))
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		_, _ = orch.Start(ctx, "bench", order{})
	}
}
