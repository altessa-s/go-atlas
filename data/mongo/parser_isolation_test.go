// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"runtime"
	"testing"
	"weak"

	"github.com/stretchr/testify/require"

	mongo "github.com/altessa-s/go-atlas/data/mongo"
)

type Embedded struct{ Value string }
type record struct {
	*Embedded
	ID string
}

func TestEmbeddedPointerTransitions(t *testing.T) {
	t.Parallel()
	for _, firstNil := range []bool{true, false} {
		t.Run(map[bool]string{true: "nil-first", false: "value-first"}[firstNil], func(t *testing.T) {
			t.Parallel()
			p := mongo.NewParser()
			for i := range 4 {
				v := record{ID: "record"}
				want := 1
				if (i%2 == 0) != firstNil {
					v.Embedded = &Embedded{Value: "present"}
					want = 2
				}
				meta := p.ParseStruct(v)
				require.Len(t, meta.Fields, want)
				clear(meta.Fields) // caller mutation must not alter the cached layout
			}
		})
	}
}

// Serial: forces garbage collection to inspect cache ownership.
func TestParserDoesNotRetainEntity(t *testing.T) {
	p := mongo.NewParser()
	type payload struct{ Data []byte }
	w := func() weak.Pointer[payload] {
		v := &payload{Data: make([]byte, 16<<20)}
		p.ParseStruct(v)
		return weak.Make(v)
	}()
	runtime.GC()
	require.Nil(t, w.Value())
	runtime.KeepAlive(p)
}
