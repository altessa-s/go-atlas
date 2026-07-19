// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestDefaultListCursorOptions(t *testing.T) {
	t.Parallel()

	opts := defaultListCursorOptions()
	require.Equal(t, DefaultListSort, opts.sort)
	require.Equal(t, bson.M{}, opts.filter)
	require.Nil(t, opts.projection)
	require.Equal(t, int64(DefaultListLimit), opts.limit)
	require.Nil(t, opts.cursor)
	require.Empty(t, opts.cursorToken)
	require.Same(t, discardLogger, opts.logger)
	require.Nil(t, opts.hint)
	require.False(t, opts.explain)
	require.Equal(t, DefaultCursorIdField, opts.cursorIdField)
	require.False(t, opts.includeTotal, "ListCursor skips total by default")
	require.Nil(t, opts.storage)
	require.Nil(t, opts.collation)
	require.Nil(t, opts.stages)
	require.Nil(t, opts.decorationStages)
	require.Empty(t, opts.subject)
}

// TestListCursorOptions covers the WithListCursor* setters. The stage
// options (WithListCursorStages, WithListCursorDecorationStages) are
// covered separately in list_cursor_pipeline_test.go.
func TestListCursorOptions(t *testing.T) {
	t.Parallel()

	customLogger := slog.New(slog.DiscardHandler)
	collation := &mongoOptions.Collation{Locale: "ru", Strength: 2}
	storage := newFakeCursorStorage()
	token := "some-cursor-token"
	cursorObj := MustNewCursor("507f1f77bcf86cd799439011")

	tests := []struct {
		name   string
		option ListCursorOption
		check  func(t *testing.T, opts *listCursorOptions)
	}{
		{
			name:   "WithListCursorLogger sets logger",
			option: WithListCursorLogger(customLogger),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Same(t, customLogger, opts.logger)
			},
		},
		{
			name:   "WithListCursorLogger ignores nil",
			option: WithListCursorLogger(nil),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Same(t, discardLogger, opts.logger)
			},
		},
		{
			name:   "WithListCursorCollation sets collation",
			option: WithListCursorCollation(collation),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Same(t, collation, opts.collation)
			},
		},
		{
			name:   "WithListCursorSort parses string",
			option: WithListCursorSort("-created_at"),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Equal(t, bson.D{{Key: "created_at", Value: SortDescending}}, opts.sort)
			},
		},
		{
			name:   "WithListCursorSort accepts bson.D verbatim",
			option: WithListCursorSort(bson.D{{Key: "name", Value: SortAscending}}),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Equal(t, bson.D{{Key: "name", Value: SortAscending}}, opts.sort)
			},
		},
		{
			name:   "WithListCursorSort empty string keeps default",
			option: WithListCursorSort(""),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Equal(t, DefaultListSort, opts.sort)
			},
		},
		{
			name:   "WithListCursorIdField sets field",
			option: WithListCursorIdField("_id"),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Equal(t, "_id", opts.cursorIdField)
			},
		},
		{
			name:   "WithListCursorIdField ignores empty string",
			option: WithListCursorIdField(""),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Equal(t, DefaultCursorIdField, opts.cursorIdField)
			},
		},
		{
			name:   "WithListCursorFilter sets filter",
			option: WithListCursorFilter(bson.M{"status": "active"}),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Equal(t, bson.M{"status": "active"}, opts.filter)
			},
		},
		{
			name:   "WithListCursorProjection sets projection",
			option: WithListCursorProjection(bson.M{"password": 0}),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Equal(t, bson.M{"password": 0}, opts.projection)
			},
		},
		{
			name:   "WithListCursorTotal true enables total",
			option: WithListCursorTotal(true),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.True(t, opts.includeTotal)
			},
		},
		{
			name:   "WithListCursorLimit sets positive limit",
			option: WithListCursorLimit(50),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Equal(t, int64(50), opts.limit)
			},
		},
		{
			name:   "WithListCursorLimit ignores zero",
			option: WithListCursorLimit(0),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Equal(t, int64(DefaultListLimit), opts.limit)
			},
		},
		{
			name:   "WithListCursorLimit ignores negative",
			option: WithListCursorLimit(-1),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Equal(t, int64(DefaultListLimit), opts.limit)
			},
		},
		{
			name:   "WithListCursorCursor string sets token",
			option: WithListCursorCursor(token),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Equal(t, token, opts.cursorToken)
				require.Nil(t, opts.cursor)
			},
		},
		{
			name:   "WithListCursorCursor *string sets token",
			option: WithListCursorCursor(&token),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Equal(t, token, opts.cursorToken)
			},
		},
		{
			name:   "WithListCursorCursor nil *string is a no-op",
			option: WithListCursorCursor[*string](nil),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Empty(t, opts.cursorToken)
				require.Nil(t, opts.cursor)
			},
		},
		{
			name:   "WithListCursorCursor *Cursor sets cursor object",
			option: WithListCursorCursor(cursorObj),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Same(t, cursorObj, opts.cursor)
				require.Empty(t, opts.cursorToken)
			},
		},
		{
			name:   "WithListCursorHint sets hint",
			option: WithListCursorHint(bson.D{{Key: "created_at", Value: 1}}),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Equal(t, bson.D{{Key: "created_at", Value: 1}}, opts.hint)
			},
		},
		{
			name:   "WithListCursorExplain enables explain",
			option: WithListCursorExplain(),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.True(t, opts.explain)
			},
		},
		{
			name:   "WithListCursorStorage sets storage",
			option: WithListCursorStorage(storage),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Same(t, storage, opts.storage.(*fakeCursorStorage))
			},
		},
		{
			name:   "WithListCursorSubject sets subject",
			option: WithListCursorSubject("tenant-A"),
			check: func(t *testing.T, opts *listCursorOptions) {
				require.Equal(t, "tenant-A", opts.subject)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := defaultListCursorOptions()
			tt.option(opts)
			tt.check(t, opts)
		})
	}
}
