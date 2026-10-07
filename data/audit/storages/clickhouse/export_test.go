// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/audit"
)

// RequireEventEqual asserts that two audit events carry the same data once
// encoded as JSON. The storage round-trips free-form maps through JSON, so
// numbers come back as float64, and timestamps come back in UTC; comparing
// the encodings, with timestamps in UTC, ignores those representation
// differences. Exported for the black-box integration tests.
func RequireEventEqual(t testing.TB, want, got *audit.Event) {
	t.Helper()
	require.JSONEq(t, eventJSON(t, want), eventJSON(t, got))
}

func eventJSON(t testing.TB, e *audit.Event) string {
	t.Helper()
	if e == nil {
		return "null"
	}
	c := *e
	c.Timestamp = c.Timestamp.UTC()
	b, err := json.Marshal(&c)
	require.NoError(t, err)
	return string(b)
}
