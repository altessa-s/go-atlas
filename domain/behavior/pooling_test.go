// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/behavior"
)

type poolInner struct {
	Secret string `behavior:"input_only"`
	Keep   string
}

type poolOuter struct {
	ID       string `behavior:"identifier"`
	Child    poolInner
	Children []poolInner
	ByKey    map[string]poolInner
}

func newPoolOuter(i int) *poolOuter {
	return &poolOuter{
		ID:       fmt.Sprintf("id-%d", i),
		Child:    poolInner{Secret: "s", Keep: fmt.Sprintf("keep-%d", i)},
		Children: []poolInner{{Keep: "a"}, {Keep: "b"}},
		ByKey:    map[string]poolInner{"m": {Keep: fmt.Sprintf("m-%d", i)}},
	}
}

// TestEngineTranslatePooling drives the pooled engine through repeated walks of
// varying values so recycled arena storage is reused across calls, and checks
// every result against the non-pooled engine.
func TestEngineTranslatePooling(t *testing.T) {
	t.Parallel()

	plain := behavior.New[[]string](pathCollector{}, behavior.WithKinds(behavior.Identifier))
	pooled := behavior.New[[]string](pathCollector{},
		behavior.WithKinds(behavior.Identifier), behavior.WithPooling())

	for i := range 50 {
		want, err := plain.Translate(t.Context(), newPoolOuter(i))
		require.NoError(t, err)

		got, err := pooled.Translate(t.Context(), newPoolOuter(i))
		require.NoError(t, err)
		require.Equal(t, want, got, "pooled run %d diverged from non-pooled result", i)
	}
}

// TestEngineTranslatePoolingConcurrent hammers one pooled engine from several
// goroutines; the race detector and the per-iteration comparison catch arenas
// leaking between concurrent walks.
func TestEngineTranslatePoolingConcurrent(t *testing.T) {
	t.Parallel()

	plain := behavior.New[[]string](pathCollector{}, behavior.WithKinds(behavior.Identifier))
	pooled := behavior.New[[]string](pathCollector{},
		behavior.WithKinds(behavior.Identifier), behavior.WithPooling())

	const workers = 8
	// One slot per worker: each goroutine writes only errs[w], and the main
	// goroutine reads after wg.Wait, so no extra synchronization is needed.
	errs := make([]error, workers)

	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := range 200 {
				v := newPoolOuter(w*1000 + i)
				want, err := plain.Translate(t.Context(), v)
				if err != nil {
					errs[w] = err
					return
				}
				got, err := pooled.Translate(t.Context(), v)
				if err != nil {
					errs[w] = err
					return
				}
				if fmt.Sprint(got) != fmt.Sprint(want) {
					errs[w] = fmt.Errorf("worker %d iteration %d: got %v, want %v", w, i, got, want)
					return
				}
			}
		})
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
}
