// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPageRequest_ClampLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit int64
		want  int64
	}{
		{"zero_defaults", 0, DefaultPageSize},
		{"negative_defaults", -1, DefaultPageSize},
		{"within_range", 50, 50},
		{"at_max", MaxPageSize, MaxPageSize},
		{"over_max_clamped", MaxPageSize + 1, MaxPageSize},
		{"one", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PageRequest{Limit: tt.limit}
			assert.Equal(t, tt.want, p.ClampLimit())
		})
	}
}

func TestEncodeDecodeTaskCursor(t *testing.T) {
	filterExpr := `status == 1`
	token := encodeTaskCursor("task-42", filterExpr)
	assert.NotEmpty(t, token)

	cur, err := decodeTaskCursor(token, filterExpr)
	require.NoError(t, err)
	assert.Equal(t, "task-42", cur.LastID)
}

func TestDecodeTaskCursor_InvalidBase64(t *testing.T) {
	_, err := decodeTaskCursor("not-valid-base64!!!", "")
	assert.ErrorIs(t, err, ErrInvalidCursor)
}

func TestDecodeTaskCursor_FilterMismatch(t *testing.T) {
	token := encodeTaskCursor("task-1", "status == 1")
	_, err := decodeTaskCursor(token, "status == 2")
	assert.ErrorIs(t, err, ErrInvalidCursor)
}

func TestDecodeTaskCursor_EmptyFilter(t *testing.T) {
	token := encodeTaskCursor("task-1", "")
	cur, err := decodeTaskCursor(token, "")
	require.NoError(t, err)
	assert.Equal(t, "task-1", cur.LastID)
}

func TestEncodeDecodeHistoryCursor(t *testing.T) {
	filterExpr := `success == true`
	token := encodeHistoryCursor(1700000000, "hist-99", filterExpr)
	assert.NotEmpty(t, token)

	cur, err := decodeHistoryCursor(token, filterExpr)
	require.NoError(t, err)
	assert.Equal(t, int64(1700000000), cur.LastStartedAt)
	assert.Equal(t, "hist-99", cur.LastID)
}

func TestDecodeHistoryCursor_InvalidBase64(t *testing.T) {
	_, err := decodeHistoryCursor("%%%invalid%%%", "")
	assert.ErrorIs(t, err, ErrInvalidCursor)
}

func TestDecodeHistoryCursor_FilterMismatch(t *testing.T) {
	token := encodeHistoryCursor(100, "h1", "filter-a")
	_, err := decodeHistoryCursor(token, "filter-b")
	assert.ErrorIs(t, err, ErrInvalidCursor)
}

func TestDecodeHistoryCursor_InvalidJSON(t *testing.T) {
	// Base64-encode something that isn't valid JSON
	_, err := decodeHistoryCursor("bm90LWpzb24", "")
	assert.ErrorIs(t, err, ErrInvalidCursor)
}

func TestFilterHash_Deterministic(t *testing.T) {
	h1 := filterHash("status == 1")
	h2 := filterHash("status == 1")
	assert.Equal(t, h1, h2)

	h3 := filterHash("status == 2")
	assert.NotEqual(t, h1, h3)
}

func TestFilterHash_EmptyString(t *testing.T) {
	h := filterHash("")
	assert.NotEmpty(t, h)
	assert.Len(t, h, 64) // SHA-256 hex is 64 chars
}
