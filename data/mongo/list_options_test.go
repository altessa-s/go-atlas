// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestDefaultListOptions(t *testing.T) {
	t.Parallel()

	opts := defaultListOptions()
	require.Equal(t, DefaultListSort, opts.sort)
	require.Equal(t, bson.M{}, opts.filter)
	require.Nil(t, opts.projection)
	require.Equal(t, int64(DefaultListLimit), opts.limit)
	require.Equal(t, int64(DefaultListOffset), opts.offset)
	require.Same(t, discardLogger, opts.logger)
	require.Nil(t, opts.hint)
	require.False(t, opts.explain)
	require.True(t, opts.includeTotal, "List computes total by default")
}

func TestListOptions(t *testing.T) {
	t.Parallel()

	customLogger := slog.New(slog.DiscardHandler)
	sortPtr := "name,-age"

	tests := []struct {
		name   string
		option ListOption
		check  func(t *testing.T, opts *listOptions)
	}{
		{
			name:   "WithListLogger sets logger",
			option: WithListLogger(customLogger),
			check: func(t *testing.T, opts *listOptions) {
				require.Same(t, customLogger, opts.logger)
			},
		},
		{
			name:   "WithListLogger ignores nil",
			option: WithListLogger(nil),
			check: func(t *testing.T, opts *listOptions) {
				require.Same(t, discardLogger, opts.logger)
			},
		},
		{
			name:   "WithListSort parses string",
			option: WithListSort("name,-age"),
			check: func(t *testing.T, opts *listOptions) {
				require.Equal(t, bson.D{
					{Key: "name", Value: SortAscending},
					{Key: "age", Value: SortDescending},
				}, opts.sort)
			},
		},
		{
			name:   "WithListSort parses *string",
			option: WithListSort(&sortPtr),
			check: func(t *testing.T, opts *listOptions) {
				require.Equal(t, bson.D{
					{Key: "name", Value: SortAscending},
					{Key: "age", Value: SortDescending},
				}, opts.sort)
			},
		},
		{
			name:   "WithListSort accepts bson.D verbatim",
			option: WithListSort(bson.D{{Key: "score", Value: SortDescending}}),
			check: func(t *testing.T, opts *listOptions) {
				require.Equal(t, bson.D{{Key: "score", Value: SortDescending}}, opts.sort)
			},
		},
		{
			name:   "WithListSort empty string keeps default",
			option: WithListSort(""),
			check: func(t *testing.T, opts *listOptions) {
				require.Equal(t, DefaultListSort, opts.sort)
			},
		},
		{
			name:   "WithListSort nil *string keeps default",
			option: WithListSort[*string](nil),
			check: func(t *testing.T, opts *listOptions) {
				require.Equal(t, DefaultListSort, opts.sort)
			},
		},
		{
			name:   "WithListFilter sets filter",
			option: WithListFilter(bson.M{"active": true}),
			check: func(t *testing.T, opts *listOptions) {
				require.Equal(t, bson.M{"active": true}, opts.filter)
			},
		},
		{
			name:   "WithListProjection sets projection",
			option: WithListProjection(bson.M{"name": 1}),
			check: func(t *testing.T, opts *listOptions) {
				require.Equal(t, bson.M{"name": 1}, opts.projection)
			},
		},
		{
			name:   "WithListTotal false disables total",
			option: WithListTotal(false),
			check: func(t *testing.T, opts *listOptions) {
				require.False(t, opts.includeTotal)
			},
		},
		{
			name:   "WithListTotal true keeps total enabled",
			option: WithListTotal(true),
			check: func(t *testing.T, opts *listOptions) {
				require.True(t, opts.includeTotal)
			},
		},
		{
			name:   "WithListLimit sets limit and offset",
			option: WithListLimit(20, 40),
			check: func(t *testing.T, opts *listOptions) {
				require.Equal(t, int64(20), opts.limit)
				require.Equal(t, int64(40), opts.offset)
			},
		},
		{
			name:   "WithListLimit ignores non-positive limit",
			option: WithListLimit(0, 40),
			check: func(t *testing.T, opts *listOptions) {
				require.Equal(t, int64(DefaultListLimit), opts.limit)
				require.Equal(t, int64(40), opts.offset)
			},
		},
		{
			name:   "WithListLimit ignores negative offset",
			option: WithListLimit(20, -1),
			check: func(t *testing.T, opts *listOptions) {
				require.Equal(t, int64(20), opts.limit)
				require.Equal(t, int64(DefaultListOffset), opts.offset)
			},
		},
		{
			// Capping is applied inside List via capListLimit/capListOffset,
			// not by the option itself — the option stores the raw value.
			name:   "WithListLimit stores values above the caps uncapped",
			option: WithListLimit(MaxListLimit+1, MaxListOffset+1),
			check: func(t *testing.T, opts *listOptions) {
				require.Equal(t, int64(MaxListLimit+1), opts.limit)
				require.Equal(t, int64(MaxListOffset+1), opts.offset)
			},
		},
		{
			name:   "WithListHint sets hint",
			option: WithListHint("user_id_1_created_at_-1"),
			check: func(t *testing.T, opts *listOptions) {
				require.Equal(t, "user_id_1_created_at_-1", opts.hint)
			},
		},
		{
			name:   "WithListExplain enables explain",
			option: WithListExplain(),
			check: func(t *testing.T, opts *listOptions) {
				require.True(t, opts.explain)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := defaultListOptions()
			tt.option(opts)
			tt.check(t, opts)
		})
	}
}
