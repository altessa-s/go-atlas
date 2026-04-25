// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldtracker_test

import (
	"fmt"
	"testing"

	"github.com/altessa-s/go-atlas/domain/fieldtracker"
)

type BenchUser struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Age     int    `json:"age"`
	Address struct {
		City   string `json:"city"`
		Street string `json:"street"`
		Zip    string `json:"zip"`
	} `json:"address"`
	Tags []string `json:"tags"`
}

func BenchmarkGetChangedFields_ReuseTracker(b *testing.B) {
	tr := fieldtracker.NewTracker()
	u1 := &BenchUser{Name: "Old", Age: 20}
	u2 := &BenchUser{Name: "New", Age: 21}

	b.ResetTimer()
	for b.Loop() {
		_ = tr.GetChangedFields(u1, u2)
	}
}

func BenchmarkGetChangedFields_OneOff(b *testing.B) {
	u1 := &BenchUser{Name: "Old", Age: 20}
	u2 := &BenchUser{Name: "New", Age: 21}

	b.ResetTimer()
	for b.Loop() {
		_ = fieldtracker.GetChangedFields(u1, u2)
	}
}

func BenchmarkGetChangedFields_LargeStruct(b *testing.B) {
	type Large struct {
		F1, F2, F3, F4, F5, F6, F7, F8, F9, F10 string
		N1, N2, N3, N4, N5, N6, N7, N8, N9, N10 int
	}

	l1 := &Large{F1: "A"}
	l2 := &Large{F1: "B", N10: 100}
	tr := fieldtracker.NewTracker()

	b.ResetTimer()
	for b.Loop() {
		_ = tr.GetChangedFields(l1, l2)
	}
}

func BenchmarkGetChangedFields_LargeSlice(b *testing.B) {
	type SliceStruct struct {
		Items []string `json:"items"`
	}

	items1 := make([]string, 1000)
	items2 := make([]string, 1000)
	for i := range items1 {
		items1[i] = fmt.Sprintf("Item %d", i)
		items2[i] = fmt.Sprintf("Item %d", i)
	}
	items2[500] = "CHANGED"

	s1 := &SliceStruct{Items: items1}
	s2 := &SliceStruct{Items: items2}
	tr := fieldtracker.NewTracker()

	b.ResetTimer()
	for b.Loop() {
		_ = tr.GetChangedFields(s1, s2)
	}
}
