// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spoolbudget_test

import (
	"strings"
	"testing"

	"github.com/altessa-s/go-atlas/core/io/spoolbudget"
)

func BenchmarkBudget_Spool(b *testing.B) {
	budget := spoolbudget.New(1 << 20)
	src := strings.Repeat("x", 1024)
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		sp, err := budget.Spool(ctx, strings.NewReader(src), 1024)
		if err != nil {
			b.Fatal(err)
		}
		_ = sp.Close()
	}
}

func BenchmarkBudget_SpoolDisabled(b *testing.B) {
	var budget *spoolbudget.Budget // disabled passthrough
	src := strings.Repeat("x", 1024)
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		sp, err := budget.Spool(ctx, strings.NewReader(src), 1024)
		if err != nil {
			b.Fatal(err)
		}
		_ = sp.Close()
	}
}
