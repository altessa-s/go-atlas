// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package base_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/secrets/internal/base"
)

func TestSingleflightGroup_Do(t *testing.T) {
	var sf base.SingleflightGroup
	var callCount atomic.Int32

	var wg sync.WaitGroup

	wg.Go(func() {
		_, _, _ = sf.Do("key", func() (any, error) {
			callCount.Add(1)
			time.Sleep(50 * time.Millisecond)
			return "result", nil
		})
	})

	wg.Go(func() {
		time.Sleep(10 * time.Millisecond)
		_, _, _ = sf.Do("key", func() (any, error) {
			callCount.Add(1)
			return "should not execute", nil
		})
	})

	wg.Wait()

	require.Equal(t, int32(1), callCount.Load())
}

func TestSingleflightGroup_CreateKey(t *testing.T) {
	var sf base.SingleflightGroup

	tests := []struct {
		name       string
		components []string
		expected   string
	}{
		{"no args", nil, ""},
		{"one arg", []string{"a"}, "a"},
		{"two args", []string{"a", "b"}, "a:b"},
		{"three args", []string{"a", "b", "c"}, "a:b:c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, sf.CreateKey(tt.components...))
		})
	}
}

func TestSingleflightGroup_Forget(t *testing.T) {
	var sf base.SingleflightGroup
	var callCount atomic.Int32

	fn := func() (any, error) {
		callCount.Add(1)
		return "result", nil
	}

	sf.Do("key", fn)
	sf.Forget("key")
	sf.Do("key", fn)

	require.Equal(t, int32(2), callCount.Load())
}

func TestDoTyped(t *testing.T) {
	var sf base.SingleflightGroup

	t.Run("correct type", func(t *testing.T) {
		result, err := base.DoTyped(&sf, "k1", func() (string, error) {
			return "typed result", nil
		})
		require.NoError(t, err)
		require.Equal(t, "typed result", result)
	})

	t.Run("with error", func(t *testing.T) {
		expectedErr := errors.New("test error")
		result, err := base.DoTyped(&sf, "k2", func() (int, error) {
			return 0, expectedErr
		})
		require.ErrorIs(t, err, expectedErr)
		require.Equal(t, 0, result)
	})
}
