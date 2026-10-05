// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jetstream_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/data/saga/storages/memory"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	sagajs "github.com/altessa-s/go-atlas/data/saga/engines/jetstream"
)

func BenchmarkSubmit(b *testing.B) {
	ns := testhelpers.StartNATSServer(b)
	_, js := testhelpers.ConnectJetStream(b, ns)
	def := saga.NewDefinition[order]("bench").
		Step("a", func(context.Context, *order) error { return nil }).ReadOnly().
		MustBuild()
	engine, err := sagajs.New(b.Context(), js, saga.New(memory.New(), def))
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if err := engine.Submit(b.Context(), strconv.Itoa(i), order{N: i}); err != nil {
			b.Fatal(err)
		}
		i++
	}
}
