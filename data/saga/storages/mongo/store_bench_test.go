// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/saga"
)

func benchInstance() *saga.Instance {
	now := time.Unix(1_700_000_000, 0).UTC()
	return &saga.Instance{
		ID:         "order-1",
		Definition: "place-order",
		Status:     saga.StatusRunning,
		Stage:      1,
		Data:       []byte(`{"n":1}`),
		Steps: []saga.StepRecord{
			{Name: "reserve", Stage: 0, Status: saga.StepCompleted, Attempts: 1, StartedAt: now, FinishedAt: now},
		},
		CreatedAt: now,
		UpdatedAt: now,
		Deadline:  now.Add(time.Minute),
		Version:   3,
	}
}

func BenchmarkToDocument(b *testing.B) {
	inst := benchInstance()
	b.ReportAllocs()
	for b.Loop() {
		_ = toDocument(inst)
	}
}

func BenchmarkFromDocument(b *testing.B) {
	doc := toDocument(benchInstance())
	b.ReportAllocs()
	for b.Loop() {
		_ = fromDocument(&doc)
	}
}
