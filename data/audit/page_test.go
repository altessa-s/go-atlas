// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/memory"
)

// An oversized limit is capped instead of overflowing the lookahead row.
func TestFetchPage_CapsLimit(t *testing.T) {
	t.Parallel()

	page, err := audit.FetchPage(t.Context(), memory.New(), nil, audit.Query{Limit: math.MaxInt})
	require.NoError(t, err)
	require.Empty(t, page.Events)
}
