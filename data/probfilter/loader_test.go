// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilter_test

import (
	"context"
	"iter"
	"testing"

	"github.com/altessa-s/go-atlas/data/probfilter"
)

func TestDataLoaderFunc(t *testing.T) {
	fn := probfilter.DataLoaderFunc(func(ctx context.Context) iter.Seq2[string, error] {
		return func(yield func(string, error) bool) {
			if !yield("a", nil) {
				return
			}
			yield("b", nil)
		}
	})

	// Count should return -1 (unknown).
	count, err := fn.Count(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if count != -1 {
		t.Errorf("Count = %d, want -1", count)
	}

	// StreamValues should yield values.
	var values []string
	for v, err := range fn.StreamValues(t.Context()) {
		if err != nil {
			t.Fatal(err)
		}
		values = append(values, v)
	}
	if len(values) != 2 {
		t.Errorf("len(values) = %d, want 2", len(values))
	}
}

func TestNewDataLoader(t *testing.T) {
	items := []string{"x", "y", "z"}
	loader := probfilter.NewDataLoader(func() iter.Seq[string] {
		return func(yield func(string) bool) {
			for _, v := range items {
				if !yield(v) {
					return
				}
			}
		}
	})

	count, _ := loader.Count(t.Context())
	if count != -1 {
		t.Errorf("Count = %d, want -1 (default)", count)
	}

	var values []string
	for v, err := range loader.StreamValues(t.Context()) {
		if err != nil {
			t.Fatal(err)
		}
		values = append(values, v)
	}
	if len(values) != 3 {
		t.Errorf("len(values) = %d, want 3", len(values))
	}
}

func TestWithCount(t *testing.T) {
	loader := probfilter.NewDataLoader(func() iter.Seq[string] {
		return func(yield func(string) bool) {}
	}, probfilter.WithCount(42))

	count, _ := loader.Count(t.Context())
	if count != 42 {
		t.Errorf("Count = %d, want 42", count)
	}
}
