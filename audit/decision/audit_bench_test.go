// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package decision_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/audit/decision"
)

type noopSink struct{}

func (noopSink) Record(context.Context, decision.Decision) error { return nil }

func BenchmarkRecorderRecordDeny(b *testing.B) {
	rec := decision.NewRecorder(noopSink{})
	ctx := b.Context()
	d := decision.Decision{Allowed: false, Subject: "u1", Action: "act", Reason: "scope_denied"}

	b.ReportAllocs()
	for b.Loop() {
		_ = rec.Record(ctx, d)
	}
}

// BenchmarkRecorderRecordAllowSkipped measures the dropped-allow fast path under
// the default deny-only policy, where no sink write happens.
func BenchmarkRecorderRecordAllowSkipped(b *testing.B) {
	rec := decision.NewRecorder(noopSink{})
	ctx := b.Context()
	d := decision.Decision{Allowed: true, Subject: "u1", Action: "act"}

	b.ReportAllocs()
	for b.Loop() {
		_ = rec.Record(ctx, d)
	}
}
