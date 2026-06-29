// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dataaudit_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/auth/audit/sinks/dataaudit"

	authaudit "github.com/altessa-s/go-atlas/auth/audit"
	coreaudit "github.com/altessa-s/go-atlas/data/audit"
)

type noopDispatcher struct{}

func (noopDispatcher) Submit(*coreaudit.Event) bool { return true }
func (noopDispatcher) Dropped() int64               { return 0 }

func BenchmarkSinkRecord(b *testing.B) {
	a, err := coreaudit.New(noopDispatcher{})
	if err != nil {
		b.Fatal(err)
	}
	if err := a.Start(); err != nil {
		b.Fatal(err)
	}
	sink := dataaudit.New(a)
	ctx := context.Background()
	d := authaudit.Decision{Allowed: false, Subject: "u1", Action: "act", Reason: "scope_denied"}

	b.ReportAllocs()
	for b.Loop() {
		_ = sink.Record(ctx, d)
	}
}
