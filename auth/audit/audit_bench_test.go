// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/auth/audit"
)

type noopSink struct{}

func (noopSink) Record(context.Context, audit.Decision) error { return nil }

func BenchmarkRecorderRecordDeny(b *testing.B) {
	rec := audit.NewRecorder(noopSink{})
	ctx := context.Background()
	d := audit.Decision{Allowed: false, Subject: "u1", Action: "act", Reason: "scope_denied"}

	b.ReportAllocs()
	for b.Loop() {
		_ = rec.Record(ctx, d)
	}
}

// BenchmarkRecorderRecordAllowSkipped measures the dropped-allow fast path under
// the default deny-only policy, where no sink write happens.
func BenchmarkRecorderRecordAllowSkipped(b *testing.B) {
	rec := audit.NewRecorder(noopSink{})
	ctx := context.Background()
	d := audit.Decision{Allowed: true, Subject: "u1", Action: "act"}

	b.ReportAllocs()
	for b.Loop() {
		_ = rec.Record(ctx, d)
	}
}
