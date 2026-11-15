// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package explain

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// White-box test covers the parser without requiring a database server.
func TestParseExplainResult(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		input    bson.M
		stage    string
		examined int64
	}{
		{"empty", bson.M{}, "UNKNOWN", 0},
		{"execution stats", bson.M{"executionStats": bson.M{"totalDocsExamined": int32(7), "winningPlan": bson.M{"stage": "COLLSCAN"}}}, "COLLSCAN", 7},
		{"planner", bson.M{"queryPlanner": bson.M{"winningPlan": bson.M{"stage": "IXSCAN", "indexName": "name_1"}}}, "IXSCAN", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseExplainResult(tc.input)
			require.Equal(t, tc.stage, got.Stage)
			require.Equal(t, tc.examined, got.DocsExamined)
			require.Equal(t, tc.stage == "IXSCAN", got.IndexUsed)
		})
	}
}
