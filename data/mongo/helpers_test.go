// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"log/slog"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestParseSortString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bson.D
	}{
		{"single asc", "name", bson.D{{Key: "name", Value: SortAscending}}},
		{"single desc", "-name", bson.D{{Key: "name", Value: SortDescending}}},
		{"multiple", "name,-age", bson.D{
			{Key: "name", Value: SortAscending},
			{Key: "age", Value: SortDescending},
		}},
		{"empty", "", nil},
		{"whitespace only", "  ,  ", nil},
		{"dash only", "-", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSortString(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i].Key != tt.want[i].Key || got[i].Value != tt.want[i].Value {
					t.Errorf("[%d] got {%s, %v}, want {%s, %v}", i, got[i].Key, got[i].Value, tt.want[i].Key, tt.want[i].Value)
				}
			}
		})
	}
}

func TestNextPowerOfTwo(t *testing.T) {
	tests := []struct {
		n    int
		want int
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{3, 4},
		{5, 8},
		{8, 8},
		{100, 128},
		{-1, 1},
	}

	for _, tt := range tests {
		got := nextPowerOfTwo(tt.n)
		if got != tt.want {
			t.Errorf("nextPowerOfTwo(%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
}

func TestCapListLimit(t *testing.T) {
	logger := slog.Default()

	if got := capListLimit(500, logger); got != 500 {
		t.Errorf("capListLimit(500) = %d, want 500", got)
	}
	if got := capListLimit(2000, logger); got != int64(MaxListLimit) {
		t.Errorf("capListLimit(2000) = %d, want %d", got, MaxListLimit)
	}
}

func TestParseSortOption(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		d, ok := parseSortOption("name,-age")
		if !ok {
			t.Fatal("expected ok")
		}
		if len(d) != 2 {
			t.Fatalf("len = %d, want 2", len(d))
		}
	})

	t.Run("empty string", func(t *testing.T) {
		_, ok := parseSortOption("")
		if ok {
			t.Error("expected not ok for empty string")
		}
	})

	t.Run("*string", func(t *testing.T) {
		s := "name"
		d, ok := parseSortOption(&s)
		if !ok {
			t.Fatal("expected ok")
		}
		if len(d) != 1 {
			t.Fatalf("len = %d, want 1", len(d))
		}
	})

	t.Run("nil *string", func(t *testing.T) {
		_, ok := parseSortOption[*string](nil)
		if ok {
			t.Error("expected not ok for nil pointer")
		}
	})

	t.Run("bson.D", func(t *testing.T) {
		input := bson.D{{Key: "name", Value: 1}}
		d, ok := parseSortOption(input)
		if !ok {
			t.Fatal("expected ok")
		}
		if d[0].Key != "name" {
			t.Errorf("got %s", d[0].Key)
		}
	})

	t.Run("empty bson.D", func(t *testing.T) {
		_, ok := parseSortOption(bson.D{})
		if ok {
			t.Error("expected not ok for empty bson.D")
		}
	})
}
