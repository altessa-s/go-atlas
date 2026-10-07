// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
)

// TestZeroFields pins the fields derived from the omitempty tags: the
// filterable ones a stored document may lack, and none it always carries.
func TestZeroFields(t *testing.T) {
	t.Parallel()

	require.Equal(t, map[string]filter.FieldKind{
		"description":     filter.FieldKindString,
		"lastRunAt":       filter.FieldKindInt,
		"nextRunAt":       filter.FieldKindInt,
		"lastRunId":       filter.FieldKindString,
		"run_started_at":  filter.FieldKindInt,
		"run_lease_until": filter.FieldKindInt,
		"run_lease_id":    filter.FieldKindString,
		"run_at":          filter.FieldKindInt,
		"skipNextRun":     filter.FieldKindBool,
		"disableHistory":  filter.FieldKindBool,
		"unmanaged":       filter.FieldKindBool,
		"oneShot":         filter.FieldKindBool,
		"revision":        filter.FieldKindInt,
	}, maps.Collect(taskZeroFields.All()))

	require.Equal(t, map[string]filter.FieldKind{
		"error": filter.FieldKindString,
	}, maps.Collect(historyZeroFields.All()))
}
