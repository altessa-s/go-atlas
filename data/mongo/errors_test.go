// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDuplicateFields_Contains(t *testing.T) {
	df := &DuplicateFields{fields: map[string]struct{}{"email": {}}}
	require.True(t, df.Contains("email"), "expected true for 'email'")
	require.False(t, df.Contains("name"), "expected false for 'name'")

	// Nil receiver
	var nilDf *DuplicateFields
	require.False(t, nilDf.Contains("email"), "expected false for nil receiver")

	// Nil fields map
	emptyDf := &DuplicateFields{}
	require.False(t, emptyDf.Contains("email"), "expected false for nil fields map")
}

func TestIsErrorDuplicate(t *testing.T) {
	// Non-duplicate error
	ok, _ := IsErrorDuplicate(errors.New("random error"))
	require.False(t, ok, "expected false for non-duplicate error")
}

func TestIsErrorCollectionNotFound(t *testing.T) {
	require.False(t, IsErrorCollectionNotFound(errors.New("random error")), "expected false for non-collection-not-found error")
}

func TestIsErrorIndexNotFound(t *testing.T) {
	require.False(t, IsErrorIndexNotFound(errors.New("random error")), "expected false for non-index-not-found error")
}

func TestIsTransientTransaction(t *testing.T) {
	// context.Canceled is NOT transient
	require.False(t, IsTransientTransaction(context.Canceled), "context.Canceled should not be transient")
	// context.DeadlineExceeded is NOT transient
	require.False(t, IsTransientTransaction(context.DeadlineExceeded), "context.DeadlineExceeded should not be transient")
	// Random error is not transient
	require.False(t, IsTransientTransaction(errors.New("random")), "random error should not be transient")
}
