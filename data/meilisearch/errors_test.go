// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsErrorIndexNotFound(t *testing.T) {
	require.False(t, IsErrorIndexNotFound(errors.New("random error")), "expected false for non-index-not-found error")
	require.False(t, IsErrorIndexNotFound(nil), "expected false for nil error")
}

func TestIsErrorIndexAlreadyExists(t *testing.T) {
	require.False(t, IsErrorIndexAlreadyExists(errors.New("random error")), "expected false for non-index-already-exists error")
	require.False(t, IsErrorIndexAlreadyExists(nil), "expected false for nil error")
}
