// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"testing"

	"github.com/altessa-s/go-atlas/observability/health"
)

func TestNew(t *testing.T) {
	coord := health.New()
	h := New(coord)
	if h == nil {
		t.Fatal("New returned nil")
	}
}
