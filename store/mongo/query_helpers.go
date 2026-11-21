// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"log/slog"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas-core/collections/slices"

	queryexplain "github.com/altessa-s/go-atlas/store/mongo/explain"
)

// getFilterFields extracts field names from a MongoDB filter for logging purposes.
// This function identifies the actual document fields being filtered, excluding MongoDB
// operators (fields starting with "$") which are query modifiers rather than field names.
//
// Parameters:
//   - filter: MongoDB query filter using bson.M syntax
//
// Returns:
//   - []string: Field names from the filter (nil if filter is empty)
//   - MongoDB operators like $or, $and, $gte are excluded
//
// Examples:
//   - bson.M{"age": 25, "name": "John"} → ["age", "name"]
//   - bson.M{"age": bson.M{"$gte": 18}} → ["age"]
//   - bson.M{"$or": [...]} → [] (empty, only operators)
//   - bson.M{} → nil
//
// Use case: Logging filter fields helps correlate queries with indexes during performance
// analysis. This is particularly useful when debugging query plans or identifying missing indexes.
func getFilterFields(filter bson.Raw) []string {
	if len(filter) == 0 {
		return nil
	}

	elements, _ := filter.Elements() // Already validated by Prepare.
	fields := make([]string, 0, len(elements))
	for _, elem := range elements {
		field := elem.Key()
		// Skip MongoDB operators that start with $
		fields = slices.AppendIf(fields, !strings.HasPrefix(field, "$"), field)
	}

	return fields
}

// executeExplainIfRequested conditionally runs explain analysis based on the explain flag.
// This function provides a clean abstraction for optional explain execution, handling
// errors gracefully by logging warnings rather than failing the query.
//
// Parameters:
//   - ctx: Context for the explain operation
//   - collection: MongoDB collection to analyze
//   - pipeline: Aggregation pipeline to explain
//   - hint: Index hint to include in the explain command (nil if none)
//   - explain: Whether to run explain (no-op if false)
//   - logger: Logger for explain output and errors
//
// Behavior:
//   - If explain is false: No-op, returns immediately
//   - If explain is true: Executes explain and logs results
//   - If explain fails: Logs warning but doesn't fail (explain errors shouldn't break queries)
//
// Error handling:
//   - Explain failures are logged as warnings with the error details
//   - The calling query continues normally even if explain fails
//   - This ensures explain is truly optional and non-intrusive
//
// Use case: Wrapper for optional explain execution that prevents explain errors from
// affecting actual query execution. Safe for production use with explain enabled.
func executeExplainIfRequested(ctx context.Context, collection *mongo.Collection, pipeline bson.A, hint any, explain bool, logger *slog.Logger) {
	if !explain {
		return
	}

	_, err := queryexplain.Run(ctx, collection, pipeline, hint, logger)
	if err != nil {
		logger.Warn("failed to execute query explain analysis", "error", err)
	}
}
