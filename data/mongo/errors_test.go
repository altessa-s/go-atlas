// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"testing"
)

func TestDuplicateFields_Contains(t *testing.T) {
	df := &DuplicateFields{fields: map[string]struct{}{"email": {}}}
	if !df.Contains("email") {
		t.Error("expected true for 'email'")
	}
	if df.Contains("name") {
		t.Error("expected false for 'name'")
	}

	// Nil receiver
	var nilDf *DuplicateFields
	if nilDf.Contains("email") {
		t.Error("expected false for nil receiver")
	}

	// Nil fields map
	emptyDf := &DuplicateFields{}
	if emptyDf.Contains("email") {
		t.Error("expected false for nil fields map")
	}
}

func TestIsErrorDuplicate(t *testing.T) {
	// Non-duplicate error
	ok, _ := IsErrorDuplicate(errors.New("random error"))
	if ok {
		t.Error("expected false for non-duplicate error")
	}
}

func TestIsErrorCollectionNotFound(t *testing.T) {
	if IsErrorCollectionNotFound(errors.New("random error")) {
		t.Error("expected false for non-collection-not-found error")
	}
}

func TestIsErrorIndexNotFound(t *testing.T) {
	if IsErrorIndexNotFound(errors.New("random error")) {
		t.Error("expected false for non-index-not-found error")
	}
}

func TestIsTransientTransaction(t *testing.T) {
	// context.Canceled is NOT transient
	if IsTransientTransaction(context.Canceled) {
		t.Error("context.Canceled should not be transient")
	}
	// context.DeadlineExceeded is NOT transient
	if IsTransientTransaction(context.DeadlineExceeded) {
		t.Error("context.DeadlineExceeded should not be transient")
	}
	// Random error is not transient
	if IsTransientTransaction(errors.New("random")) {
		t.Error("random error should not be transient")
	}
}
