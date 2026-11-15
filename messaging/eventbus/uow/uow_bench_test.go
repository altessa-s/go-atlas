// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uow_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/messaging/eventbus/uow"
	"github.com/altessa-s/go-atlas/messaging/eventbus/uow/uowtest"
)

func BenchmarkRun_TwoEffects(b *testing.B) {
	r := uow.New(&fakeCommitter{}, nil)
	noop := func(context.Context) error { return nil }
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		_ = r.Run(ctx, func(c context.Context) error {
			_ = uow.OnCommit(c, uow.Effect{Label: "a", Apply: noop, Compensate: noop})
			_ = uow.OnCommit(c, uow.Effect{Label: "b", Apply: noop, Compensate: noop})
			return nil
		})
	}
}

func BenchmarkRun_TwoHooksWithDetector(b *testing.B) {
	f := &uowtest.Fake{}
	r := uow.New(f, nil, uow.WithDetector(uowtest.Transaction))
	noop := func(context.Context) {}
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		_ = r.Run(ctx, func(c context.Context) error {
			_ = r.AfterCommit(c, noop)
			_ = r.AfterCommit(c, noop)
			return nil
		})
	}
}

func BenchmarkRun_Nested(b *testing.B) {
	f := &uowtest.Fake{}
	r := uow.New(f, nil, uow.WithDetector(uowtest.Transaction))
	noop := func(context.Context) {}
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		_ = r.Run(ctx, func(c context.Context) error {
			return r.Run(c, func(nc context.Context) error {
				return r.AfterCommit(nc, noop)
			})
		})
	}
}

func BenchmarkAfterCommit_OutsideTransaction(b *testing.B) {
	f := &uowtest.Fake{}
	r := uow.New(f, nil, uow.WithDetector(uowtest.Transaction), uow.WithRunHooksOutsideTransaction())
	noop := func(context.Context) {}
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		_ = r.AfterCommit(ctx, noop)
	}
}

func BenchmarkRun_NoEffects(b *testing.B) {
	r := uow.New(&fakeCommitter{}, nil)
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		_ = r.Run(ctx, func(context.Context) error { return nil })
	}
}
