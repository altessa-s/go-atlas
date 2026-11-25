// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authaudit_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/audit/decision"
	"github.com/altessa-s/go-atlas/transport/internal/authaudit"
)

func BenchmarkRecord(b *testing.B) {
	sink := decision.SinkFunc(func(context.Context, decision.Decision) error { return nil })
	ctx := b.Context()
	for b.Loop() {
		_ = authaudit.Record(ctx, sink, "grpc", "authenticate", "user", true, "")
	}
}
