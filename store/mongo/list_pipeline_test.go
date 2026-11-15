// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestBuildPipelinePreservesSortDefaults(t *testing.T) {
	t.Parallel()
	for _, sort := range []bson.D{nil, {}, {{Key: "name", Value: 1}}} {
		opts := defaultListOptions()
		opts.sort = sort
		got := buildPipeline(opts)
		expected := sort
		if len(expected) == 0 {
			expected = DefaultListSort
		}
		require.Equal(t, expected, got[0].(bson.M)["$sort"])
		require.Equal(t, sort, opts.sort)
	}
}
