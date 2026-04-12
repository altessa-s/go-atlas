// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package concurrency_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
)

func TestProcessCollect_Ordering(t *testing.T) {
	// Create a list of integers
	count := 100
	input := make([]int, count)
	for i := range count {
		input[i] = i
	}

	// Process them with random delays to ensure out-of-order completion
	ctx := t.Context()
	results, err := concurrency.ProcessCollect(ctx, input, func(ctx context.Context, item int) (int, error) {
		// Random delay between 0 and 10ms
		time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
		return item, nil
	}, concurrency.WithConcurrency[int](10))

	require.NoError(t, err)
	require.Equal(t, input, results, "ProcessCollect did not preserve order")
}
