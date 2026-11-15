// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/audit/decision"
)

type migrationRecorder struct{}

func (migrationRecorder) Record(context.Context, decision.Decision) error { return nil }

func TestWithAuditBaseline(t *testing.T) {
	got := defaultOptions()
	WithAudit(nil, nil)(got)
	if got.recorder != nil || got.subjectOf != nil {
		t.Fatal("nil audit arguments were changed")
	}
	rec := migrationRecorder{}
	subject := func(any) string { return "subject" }
	WithAudit(rec, subject)(got)
	if got.recorder == nil || got.subjectOf == nil || got.subjectOf(nil) != "subject" {
		t.Fatal("audit fields were not both applied")
	}
}
