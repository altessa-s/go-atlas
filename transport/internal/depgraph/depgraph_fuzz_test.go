// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package depgraph

import (
	"log/slog"
	"testing"
)

func FuzzBuild(f *testing.F) {
	f.Add("a", "b", "c", true)
	f.Add("x", "y", "z", false)

	f.Fuzz(func(t *testing.T, name1, name2, name3 string, depOn1 bool) {
		logger := slog.New(slog.DiscardHandler)

		var deps []string
		if depOn1 {
			deps = []string{name1}
		}

		items := []*fuzzItem{
			{name: name1},
			{name: name2, deps: deps},
			{name: name3},
		}

		// Should not panic
		Build(items, logger)
	})
}

type fuzzItem struct {
	name string
	deps []string
}

func (f *fuzzItem) Name() string           { return f.name }
func (f *fuzzItem) Dependencies() []string { return f.deps }
