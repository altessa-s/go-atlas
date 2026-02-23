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
func getFilterFields(filter bson.M) []string {
	if len(filter) == 0 {
		return nil
	}

	fields := make([]string, 0, len(filter))
	for field := range filter {
		// Skip MongoDB operators that start with $
		if !strings.HasPrefix(field, "$") {
			fields = append(fields, field)
		}
	}

	return fields
}

// executeExplain runs an aggregation pipeline with MongoDB's explain command for performance analysis.
// This function executes the query plan analysis to understand how MongoDB will execute the query,
// including index usage, execution time, and documents examined.
//
// The explain output provides crucial performance insights:
//   - Whether indexes are being used (IXSCAN vs COLLSCAN)
//   - Query execution time in milliseconds
//   - Number of documents examined vs returned (selectivity ratio)
//   - Index names and multikey status
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - collection: The MongoDB collection to analyze
//   - pipeline: Aggregation pipeline to explain
//   - logger: Logger for debug output and errors
//
// Returns:
//   - *ExplainStats: Parsed performance metrics from explain output
//   - error: If the explain command fails or explain result cannot be decoded
//
// The function:
//  1. Constructs explain command with "executionStats" verbosity
//  2. Executes via database.RunCommand (correct method for aggregation explains)
//  3. Logs raw explain result at debug level
//  4. Parses explain output into structured ExplainStats
//  5. Logs performance analysis with recommendations
//
// Use case: Performance troubleshooting, query optimization validation, regression testing.
// Enable explain during development to catch performance issues before production.
func executeExplain(ctx context.Context, collection *mongo.Collection, pipeline bson.A, logger *slog.Logger) (*ExplainStats, error) {
	// Use the database RunCommand with explain for aggregation
	// This is the correct way to get explain output for aggregation pipelines in v2
	explainCmd := bson.M{
		"explain": bson.M{
			"aggregate": collection.Name(),
			"pipeline":  pipeline,
		},
		"verbosity": "executionStats",
	}

	var explainResult bson.M
	err := collection.Database().RunCommand(ctx, explainCmd).Decode(&explainResult)
	if err != nil {
		// Log the error with more context for debugging
		logger.Error("failed to execute explain command",
			"error", err,
			"collection", collection.Name(),
			"command", explainCmd,
		)
		return nil, err
	}

	// Log raw explain result for debugging (at debug level)
	logger.Debug("raw explain result", "explain_output", explainResult)

	// Extract performance statistics from explain result
	stats := parseExplainResult(explainResult)

	// Log performance analysis
	logExplainStats(ctx, stats, logger)

	return stats, nil
}

// parseExplainResult extracts performance metrics from MongoDB's explain command output.
// MongoDB explain output structure varies by version and query type, so this function
// handles multiple possible formats to reliably extract execution statistics.
//
// Supported explain output structures:
//  1. Aggregation with stages: explainResult["stages"][0]["$cursor"]["executionStats"]
//  2. Direct executionStats: explainResult["executionStats"]
//  3. Query planner only: explainResult["queryPlanner"]["winningPlan"]
//
// Parameters:
//   - explainResult: Raw explain output from MongoDB as bson.M
//
// Returns:
//   - *ExplainStats: Structured performance metrics with defaults for missing data
//
// Extracted metrics:
//   - IndexUsed: Whether an index was used (IXSCAN) or collection scan (COLLSCAN)
//   - IndexName: Name of the index used (if any)
//   - Stage: Execution plan stage type
//   - DocsExamined: Total documents examined during execution
//   - DocsReturned: Documents returned by the query
//   - ExecutionTimeMillis: Query execution time
//   - KeysExamined: Number of index entries examined
//   - IsMultiKey: Whether the index is multikey
//
// Default values (when data is unavailable):
//   - Stage: "UNKNOWN"
//   - IndexUsed: false
//   - Other numeric fields: 0
//
// Use case: Converting MongoDB's complex, nested explain output into a simple,
// structured format suitable for logging and performance analysis.
func parseExplainResult(explainResult bson.M) *ExplainStats {
	stats := &ExplainStats{
		Stage:     "UNKNOWN",
		IndexUsed: false,
	}

	// Navigate through the explain structure to find execution stats
	// The structure can vary depending on MongoDB version and explain method

	// Try to find stages in the explain output
	if stages, ok := explainResult["stages"].(bson.A); ok && len(stages) > 0 {
		if firstStage, ok := stages[0].(bson.M); ok {
			// Look for $cursor stage which contains execution stats
			if cursor, ok := firstStage["$cursor"].(bson.M); ok {
				if executionStats, ok := cursor["executionStats"].(bson.M); ok {
					parseExecutionStats(executionStats, stats)
				}
			}
		}
	} else if executionStats, ok := explainResult["executionStats"].(bson.M); ok {
		// Direct executionStats in the root (alternative structure)
		parseExecutionStats(executionStats, stats)
	} else if queryPlanner, ok := explainResult["queryPlanner"].(bson.M); ok {
		// Parse from queryPlanner section if executionStats not available
		if winningPlan, ok := queryPlanner["winningPlan"].(bson.M); ok {
			parseWinningPlan(winningPlan, stats)
		}
	}

	return stats
}

// parseExecutionStats extracts detailed execution statistics from the executionStats section.
// This function handles the core performance metrics from MongoDB's explain output,
// including document counts, execution time, and winning plan details.
//
// MongoDB can return statistics in different numeric types (int32 or int64), so this
// function safely handles both formats using type assertions with fallback.
//
// Parameters:
//   - executionStats: The executionStats section from explain output
//   - stats: Pointer to ExplainStats struct to populate (modified in place)
//
// Extracted fields:
//   - totalDocsExamined: Total documents scanned (int64 or int32)
//   - nReturned: Number of documents returned (int64 or int32)
//   - executionTimeMillis: Query execution time in milliseconds (int64 or int32)
//   - winningPlan: Nested structure containing index usage details
//
// The function modifies the stats parameter by setting:
//   - stats.DocsExamined
//   - stats.DocsReturned
//   - stats.ExecutionTimeMillis
//   - stats.IndexUsed, stats.IndexName (via parseWinningPlan)
//
// Type handling: Tries int64 first, falls back to int32, leaves unchanged if neither matches.
//
// Use case: Core metrics extraction for query performance monitoring and optimization analysis.
func parseExecutionStats(executionStats bson.M, stats *ExplainStats) {
	// Extract basic execution metrics
	if totalDocsExamined, ok := executionStats["totalDocsExamined"].(int64); ok {
		stats.DocsExamined = totalDocsExamined
	} else if totalDocsExamined, ok := executionStats["totalDocsExamined"].(int32); ok {
		stats.DocsExamined = int64(totalDocsExamined)
	}

	if nReturned, ok := executionStats["nReturned"].(int64); ok {
		stats.DocsReturned = nReturned
	} else if nReturned, ok := executionStats["nReturned"].(int32); ok {
		stats.DocsReturned = int64(nReturned)
	}

	if executionTimeMillis, ok := executionStats["executionTimeMillis"].(int64); ok {
		stats.ExecutionTimeMillis = executionTimeMillis
	} else if executionTimeMillis, ok := executionStats["executionTimeMillis"].(int32); ok {
		stats.ExecutionTimeMillis = int64(executionTimeMillis)
	}

	// Extract winning plan information
	if winningPlan, ok := executionStats["winningPlan"].(bson.M); ok {
		parseWinningPlan(winningPlan, stats)
	}
}

// parseWinningPlan extracts index usage information from MongoDB's winning execution plan.
// The winning plan represents the query execution strategy MongoDB selected after
// evaluating multiple alternatives. This function identifies the plan's stage type
// and extracts index-related details.
//
// Plan stages and their meaning:
//   - IXSCAN: Index scan - query uses an index (efficient)
//   - COLLSCAN: Collection scan - full table scan (inefficient for large collections)
//   - FETCH: Document retrieval stage, often follows IXSCAN
//   - Others: Various stages that may have nested inputStage
//
// Parameters:
//   - winningPlan: The winningPlan section from explain output (nested structure)
//   - stats: Pointer to ExplainStats struct to populate (modified in place)
//
// For IXSCAN stages, extracts:
//   - indexName: Name of the index being used
//   - keysExamined: Number of index entries scanned (int64 or int32)
//   - isMultiKey: Whether this is a multikey index (affects performance)
//
// Recursive handling:
//   - If stage is not IXSCAN/COLLSCAN, checks for nested inputStage
//   - Recursively parses inputStage to find the actual scan operation
//   - Handles complex plans like FETCH → IXSCAN
//
// The function modifies the stats parameter by setting:
//   - stats.Stage
//   - stats.IndexUsed (true for IXSCAN, false for COLLSCAN)
//   - stats.IndexName (for IXSCAN only)
//   - stats.KeysExamined (for IXSCAN only)
//   - stats.IsMultiKey (for IXSCAN only)
//
// Use case: Determining whether queries are using indexes and identifying which
// indexes are being used for query optimization and troubleshooting.
func parseWinningPlan(winningPlan bson.M, stats *ExplainStats) {
	if stage, ok := winningPlan["stage"].(string); ok {
		stats.Stage = stage

		// Determine if index was used based on stage type
		switch stage {
		case "IXSCAN":
			stats.IndexUsed = true
			// Extract index name
			if indexName, ok := winningPlan["indexName"].(string); ok {
				stats.IndexName = indexName
			}
			// Extract keys examined
			if keysExamined, ok := winningPlan["keysExamined"].(int64); ok {
				stats.KeysExamined = keysExamined
			} else if keysExamined, ok := winningPlan["keysExamined"].(int32); ok {
				stats.KeysExamined = int64(keysExamined)
			}
			// Check if multikey index
			if isMultiKey, ok := winningPlan["isMultiKey"].(bool); ok {
				stats.IsMultiKey = isMultiKey
			}
		case "COLLSCAN":
			stats.IndexUsed = false
		default:
			// Check for nested stages (e.g., FETCH -> IXSCAN)
			if inputStage, ok := winningPlan["inputStage"].(bson.M); ok {
				parseWinningPlan(inputStage, stats)
			}
		}
	}
}

// logExplainStats logs query performance analysis with actionable optimization recommendations.
// This function analyzes explain statistics to identify performance issues and suggests
// improvements. Log level is automatically adjusted based on performance characteristics.
//
// Log level determination:
//   - WARN: Collection scan detected (no index used)
//   - WARN: High examination ratio (>10:1, poor selectivity)
//   - INFO: Normal execution (index used with good selectivity)
//
// Logged metrics (always included):
//   - index_used: Boolean indicating index usage
//   - stage: Execution plan stage (IXSCAN, COLLSCAN, etc.)
//   - index_name: Name of index used (if applicable)
//   - docs_examined: Total documents examined
//   - docs_returned: Documents returned to client
//   - execution_time_ms: Query execution time in milliseconds
//   - keys_examined: Index entries scanned (if applicable)
//   - is_multi_key: Whether index is multikey (if applicable)
//
// Performance warnings and recommendations:
//
//  1. Collection scan (IndexUsed=false):
//     - Warning: "performance alert: collection scan detected"
//     - Recommendation: "consider adding an index for the query filter fields"
//     - Impact: "high - query performance will degrade with collection size"
//
//  2. High examination ratio (>10:1):
//     - Warning: "performance alert: low query selectivity"
//     - Includes: docs_examined, docs_returned, selectivity_ratio
//     - Recommendation: "consider optimizing query filters or index design"
//
// Parameters:
//   - ctx: Context for logging
//   - stats: Parsed explain statistics
//   - logger: Logger for output
//
// Use case: Proactive performance monitoring and optimization guidance. Developers
// can enable explain in staging/development to catch performance issues before production.
func logExplainStats(ctx context.Context, stats *ExplainStats, logger *slog.Logger) {
	logLevel := slog.LevelInfo

	// Determine log level based on performance characteristics
	if !stats.IndexUsed {
		logLevel = slog.LevelWarn
	} else if stats.DocsExamined > 0 && stats.DocsReturned > 0 {
		// Check selectivity ratio - if we examine many more docs than we return, it might be inefficient
		ratio := float64(stats.DocsExamined) / float64(stats.DocsReturned)
		if ratio > HighExaminationRatio { // More than 10:1 examination ratio
			logLevel = slog.LevelWarn
		}
	}

	// Log the statistics
	logger.Log(ctx, logLevel, "query execution plan analysis",
		"index_used", stats.IndexUsed,
		"stage", stats.Stage,
		"index_name", stats.IndexName,
		"docs_examined", stats.DocsExamined,
		"docs_returned", stats.DocsReturned,
		"execution_time_ms", stats.ExecutionTimeMillis,
		"keys_examined", stats.KeysExamined,
		"is_multi_key", stats.IsMultiKey,
	)

	// Provide optimization recommendations
	if !stats.IndexUsed {
		logger.Warn("performance alert: collection scan detected",
			"recommendation", "consider adding an index for the query filter fields",
			"impact", "high - query performance will degrade with collection size",
		)
	} else if stats.DocsExamined > 0 && stats.DocsReturned > 0 {
		ratio := float64(stats.DocsExamined) / float64(stats.DocsReturned)
		if ratio > HighExaminationRatio {
			logger.Warn("performance alert: low query selectivity",
				"docs_examined", stats.DocsExamined,
				"docs_returned", stats.DocsReturned,
				"selectivity_ratio", ratio,
				"recommendation", "consider optimizing query filters or index design",
			)
		}
	}
}

// executeExplainIfRequested conditionally runs explain analysis based on the explain flag.
// This function provides a clean abstraction for optional explain execution, handling
// errors gracefully by logging warnings rather than failing the query.
//
// Parameters:
//   - ctx: Context for the explain operation
//   - collection: MongoDB collection to analyze
//   - pipeline: Aggregation pipeline to explain
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
func executeExplainIfRequested(ctx context.Context, collection *mongo.Collection, pipeline bson.A, explain bool, logger *slog.Logger) {
	if !explain {
		return
	}

	_, err := executeExplain(ctx, collection, pipeline, logger)
	if err != nil {
		logger.Warn("failed to execute query explain analysis", "error", err)
	}
}
