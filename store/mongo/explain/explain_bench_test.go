// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package explain

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func BenchmarkParseExplainResult(b *testing.B) {
	doc := bson.M{
		"queryPlanner": bson.M{
			"winningPlan": bson.M{
				"stage": "FETCH",
				"inputStage": bson.M{
					"stage":     "IXSCAN",
					"indexName": "email_1",
				},
			},
		},
		"executionStats": bson.M{
			"totalDocsExamined":   int64(10),
			"nReturned":           int64(5),
			"executionTimeMillis": int64(1),
			"totalKeysExamined":   int64(10),
		},
	}
	for b.Loop() {
		_ = parseExplainResult(doc)
	}
}
