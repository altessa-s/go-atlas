// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/core/collections/slices"
)

// Default pagination constants
const (
	// DefaultListLimit is the default number of items to return in paginated queries
	DefaultListLimit = 100
	// DefaultListOffset is the default starting offset for paginated queries
	DefaultListOffset = 0
	// MaxListLimit is the maximum allowed number of items to return in a single query
	// This prevents memory issues and ensures reasonable response times
	MaxListLimit = 1000
)

// DefaultListSort is the default sort order for list operations
var DefaultListSort = bson.D{{Key: "created_at", Value: -1}}

// Performance analysis constants
const (
	// HighExaminationRatio is the threshold ratio for high document examination
	// When an examined/returned ratio exceeds this value, it indicates poor query performance
	HighExaminationRatio = 10
)

// listOptions holds configuration for offset-based pagination queries.
// This struct contains all parameters needed to execute paginated queries using
// the traditional offset/limit approach. While simpler than cursor-based pagination,
// offset-based pagination has O(n) performance characteristics that degrade for deep pages.
//
// The options control filtering, sorting, projection, pagination boundaries, and query
// optimization features including index hints and performance analysis via explain.
type listOptions struct {
	sort         bson.D
	filter       bson.M
	projection   bson.M
	limit        int64
	offset       int64
	logger       *slog.Logger
	hint         any  // Index hint for query optimization
	explain      bool // Enable query execution plan analysis
	includeTotal bool
}

// defaultListOptions returns default configuration for offset-based pagination.
// This function initializes sensible defaults suitable for most use cases:
//   - Sort: DefaultListSort (created_at descending)
//   - Filter: Empty (no filtering)
//   - Limit: DefaultListLimit (100 items per page)
//   - Offset: DefaultListOffset (0, start from beginning)
//   - Logger: DiscardLogger (no logging)
//   - includeTotal: true (compute total count)
//
// These defaults can be overridden using functional options like WithListSort,
// WithListFilter, WithListLimit, etc.
func defaultListOptions() *listOptions {
	return &listOptions{
		sort:         DefaultListSort,
		filter:       bson.M{},
		limit:        DefaultListLimit,
		offset:       DefaultListOffset,
		logger:       slog.New(slog.DiscardHandler),
		includeTotal: true,
	}
}

// ListOption represents a functional option for configuring list operations.
type ListOption func(*listOptions)

// WithListLogger configures a custom logger for list operations.
// This is useful for monitoring and debugging query performance.
//
// Parameters:
//   - logger: Custom structured logger instance
//
// Example:
//   - WithLogger(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
func WithListLogger(logger *slog.Logger) ListOption {
	return func(options *listOptions) {
		if logger != nil {
			options.logger = logger
		}
	}
}

// WithListSort configures the sort order for list operations.
// It accepts string, *string, or bson.D types for maximum flexibility.
//
// Parameters:
//   - sort: Sort specification (string like "name,-age", *string, or bson.D)
//
// String format:
//   - Field names separated by commas
//   - Prefix with "-" for descending order (e.g., "name,-created_at")
//
// Example:
//   - WithListSort("name,-created_at") // name ascending, created_at descending
//   - WithListSort(bson.D{{"name", 1}, {"created_at", -1}})
func WithListSort[T ~string | ~*string | bson.D](sort T) ListOption {
	return func(options *listOptions) {
		if parsed, ok := parseSortOption(sort); ok {
			options.sort = parsed
		}
	}
}

// WithListFilter configures the query filter for list operations.
// The filter follows MongoDB query syntax for matching documents.
//
// Parameters:
//   - filter: MongoDB query filter using bson.M syntax
//
// Example:
//   - WithListFilter(bson.M{"active": true})
//   - WithListFilter(bson.M{"age": bson.M{"$gte": 18}})
func WithListFilter(filter bson.M) ListOption {
	return func(options *listOptions) {
		options.filter = filter
	}
}

// WithListProjection configures field projection for list operations.
// This controls which fields are included or excluded from the results.
//
// Parameters:
//   - projection: MongoDB projection specification using bson.M syntax
//
// Example:
//   - WithListProjection(bson.M{"name": 1, "email": 1}) // include only name and email
//   - WithListProjection(bson.M{"password": 0}) // exclude password field
func WithListProjection(projection bson.M) ListOption {
	return func(options *listOptions) {
		options.projection = projection
	}
}

// WithListTotal controls whether the total count is computed.
// Disabling the total avoids running an additional $count stage, which can dramatically
// improve performance on large collections when the caller does not need the total.
//
// Parameters:
//   - include: Set to false to skip total computation (ListResult.Total will be -1)
func WithListTotal(include bool) ListOption {
	return func(options *listOptions) {
		options.includeTotal = include
	}
}

// WithListLimit configures pagination for list operations.
// It sets both the maximum number of items to return and the offset.
// The limit is automatically capped at MaxListLimit to prevent memory issues.
//
// Parameters:
//   - limit: Maximum number of items to return (must be > 0, capped at MaxListLimit)
//   - offset: Number of items to skip (must be >= 0)
//
// Example:
//   - WithListLimit(20, 0) // First page: items 1-20
//   - WithListLimit(20, 20) // Second page: items 21-40
//   - WithListLimit(2000, 0) // Automatically capped to MaxListLimit (1000)
func WithListLimit(limit, offset int64) ListOption {
	return func(options *listOptions) {
		if limit > 0 {
			options.limit = limit
		}

		if offset >= 0 {
			options.offset = offset
		}
	}
}

// WithListHint provides an index hint to optimize query execution.
// This forces MongoDB to use a specific index, which can improve performance
// when the query planner doesn't choose the optimal index automatically.
//
// Parameters:
//   - hint: Index specification (string name, bson.D keys, or bson.M structure)
//
// Supported hint formats:
//   - String: Index name (e.g., "user_id_1_created_at_-1")
//   - bson.D: Index keys (e.g., bson.D{{"user_id", 1}, {"created_at", -1}})
//   - bson.M: Index structure (e.g., bson.M{"user_id": 1, "created_at": -1})
//
// Example:
//   - WithListHint("user_id_1_created_at_-1") // Use named index
//   - WithListHint(bson.D{{"status", 1}, {"created_at", -1}}) // Use compound index
//   - WithListHint(bson.M{"email": 1}) // Use single field index
//
// Warning: Use hints carefully as they override MongoDB's query optimizer.
// Inappropriate hints can significantly degrade performance.
func WithListHint(hint any) ListOption {
	return func(options *listOptions) {
		options.hint = hint
	}
}

// WithListExplain enables query execution plan analysis for performance monitoring.
// When enabled, the query execution plan is analyzed to validate index usage
// and identify potential performance issues.
//
// This option enables:
//   - Index usage validation (warns about collection scans)
//   - Execution statistics logging
//   - Performance recommendations based on explain output
//   - Query optimization insights
//
// Example:
//   - WithListExplain() // Enable explain analysis
//
// Note: Enabling explain adds minimal overhead but provides valuable insights
// for query optimization and performance monitoring.
func WithListExplain() ListOption {
	return func(options *listOptions) {
		options.explain = true
	}
}

// ListResult represents the result of a paginated list operation.
// It contains both the items and the total count.
type ListResult[T any] struct {
	// Items contains the actual data items returned from the query
	Items []T `bson:"items" json:"items"`
	// Total contains the total number of items available (before pagination).
	// A value of -1 indicates the total was not computed.
	Total int64 `bson:"total" json:"total"`
}

// ExplainStats contains key performance metrics from MongoDB explain output.
type ExplainStats struct {
	// IndexUsed indicates whether an index was used (true) or collection scan was performed (false)
	IndexUsed bool `json:"index_used"`
	// IndexName is the name of the index used, empty if no index used
	IndexName string `json:"index_name,omitempty"`
	// Stage is the winning execution plan stage (e.g., "IXSCAN", "COLLSCAN")
	Stage string `json:"stage"`
	// DocsExamined is the number of documents examined during execution
	DocsExamined int64 `json:"docs_examined"`
	// DocsReturned is the number of documents returned by the query
	DocsReturned int64 `json:"docs_returned"`
	// ExecutionTimeMillis is the total execution time in milliseconds
	ExecutionTimeMillis int64 `json:"execution_time_millis"`
	// IsMultiKey indicates if the index used is multikey (can impact performance)
	IsMultiKey bool `json:"is_multi_key,omitempty"`
	// KeysExamined is the number of index keys examined
	KeysExamined int64 `json:"keys_examined,omitempty"`
}

// buildPipeline constructs an optimized MongoDB aggregation pipeline for offset-based pagination.
// The pipeline is carefully ordered to maximize performance through proper index utilization
// and early filtering to reduce the dataset size before expensive operations.
//
// Pipeline stages (in order):
//  1. $hint (optional): Forces MongoDB to use a specific index for optimal query performance
//  2. $match: Filters documents early to reduce dataset size (most selective operation first)
//  3. $sort: Orders filtered results by specified fields (benefits from compound indexes)
//  4. $facet: Splits into parallel pipelines for efficiency
//     - items: $skip (offset) + $limit + $project (optional field projection)
//     - count: $count (total documents matching filter, skipped if includeTotal is false)
//  5. $unwind + $project: Transforms facet output into {items: [], total: N} structure
//
// The facet stage enables fetching both paginated items and total count in a single database
// round-trip, significantly improving performance compared to separate queries.
//
// Parameters:
//   - opts: Configuration options including filter, sort, limit, offset, projection, etc.
//
// Returns:
//   - bson.A: Complete aggregation pipeline ready for execution
//
// Performance considerations:
//   - $match before $sort enables index usage for filtering
//   - $skip/$limit applied after sort for correct pagination
//   - $facet provides parallelization of items and count queries
//   - Optional projection reduces network traffic by limiting returned fields
//
// Performance characteristics:
//   - O(offset + limit) complexity - degrades linearly with offset depth
//   - For deep pagination (large offsets), consider cursor-based pagination instead
func buildPipeline(opts *listOptions) bson.A {
	pipeline := bson.A{}

	// 1. HINT (OPTIONAL) - Force specific index usage
	// This should come before $match to ensure the hint is applied to filtering
	pipeline = slices.AppendIf[any](pipeline, opts.hint != nil, bson.M{"$hint": opts.hint})

	// 2. MATCH FIRST - Filter early to reduce the dataset size
	// This is the most selective operation and should come after hint
	pipeline = slices.AppendIf[any](pipeline, len(opts.filter) > 0, bson.M{"$match": opts.filter})

	// 3. SORT AFTER FILTERING - Better index utilization when combined with match
	sortStage := opts.sort
	if len(sortStage) == 0 {
		sortStage = DefaultListSort
	}
	pipeline = append(pipeline, bson.M{"$sort": sortStage})

	// 4. FACET - Split into parallel pipelines for items and count
	// This allows us to get both paginated results and total count efficiently
	itemsPipeline := bson.A{}

	// Add skip/limit for pagination in the items branch
	itemsPipeline = slices.AppendIf[any](itemsPipeline, opts.offset > 0, bson.M{"$skip": opts.offset})
	itemsPipeline = append(itemsPipeline, bson.M{"$limit": opts.limit})

	// Add projection if specified (after pagination to reduce data transfer)
	itemsPipeline = slices.AppendIf[any](itemsPipeline, opts.projection != nil, bson.M{"$project": opts.projection})

	// Facet stage with optimized branches
	facetStage := bson.M{
		"items": itemsPipeline,
	}
	if opts.includeTotal {
		facetStage["count"] = bson.A{bson.M{"$count": "total"}}
	}

	pipeline = append(pipeline, bson.M{"$facet": facetStage})

	// 5. UNWIND AND PROJECT - Transform facet results into final structure
	return appendFacetResultTransform(pipeline, opts.includeTotal)
}

// List performs a paginated query on the collection and returns items with total count.
// The result type T should be the item type (not a wrapper struct).
//
// This function supports comprehensive query optimization through various options:
//   - Filtering with MongoDB query syntax
//   - Sorting with multiple fields and directions
//   - Field projection to reduce network traffic
//   - Index hints for query performance optimization
//   - Configurable pagination limits with safety caps
//
// Index Hint Examples:
//
//	// Use a named compound index for user queries
//	result, err := List[User](ctx, userCollection,
//	    WithListFilter(bson.M{"active": true}),
//	    WithListHint("user_active_1_created_at_-1"),
//	    WithListLimit(50, 0),
//	)
//
//	// Force usage of specific index fields for time-range queries
//	result, err := List[Order](ctx, orderCollection,
//	    WithListFilter(bson.M{"date": bson.M{"$gte": startDate, "$lt": endDate}}),
//	    WithListHint(bson.D{{"date", -1}, {"status", 1}}),
//	    WithListSort("date,-amount"),
//	)
//
//	// Use single field index for simple lookups
//	result, err := List[Product](ctx, productCollection,
//	    WithListFilter(bson.M{"category": "electronics"}),
//	    WithListHint(bson.M{"category": 1}),
//	    WithListProjection(bson.M{"name": 1, "price": 1}),
//	)
//
// Query Performance Analysis Examples:
//
//	// Enable explain to validate index usage and identify performance issues
//	result, err := List[Order](ctx, orderCollection,
//	    WithListFilter(bson.M{"status": "pending", "created_at": bson.M{"$gte": yesterday}}),
//	    WithListSort("-created_at"),
//	    WithListExplain(), // Enables performance analysis
//	    WithListLogger(logger), // Required for explain output
//	)
//	// This will log index usage, execution time, and performance recommendations
//
//	// Combine explain with hints to validate optimization effectiveness
//	result, err := List[User](ctx, userCollection,
//	    WithListFilter(bson.M{"active": true, "department": "engineering"}),
//	    WithListHint("user_active_1_department_1_created_at_-1"),
//	    WithListExplain(), // Validate that the hint is effective
//	)
//
// Performance Tips:
//   - Use hints when you know the optimal index for your query pattern
//   - Enable explain to monitor query performance and validate optimizations
//   - Monitor query performance with MongoDB profiler before and after adding hints
//   - Consider compound indexes that match your filter + sort pattern
//   - Be cautious with hints - they override MongoDB's query optimizer
//   - Use explain regularly in development to catch performance regressions
//
// Basic Example:
//
//	type User struct { Name string `bson:"name"` }
//	result, err := List[User](ctx, collection, WithListLimit(10, 0))
//	// result.Items contains []User, result.Total contains total count
func List[T any](ctx context.Context, collection *mongo.Collection, o ...ListOption) (*ListResult[T], error) {
	opts := defaultListOptions()
	for _, opt := range o {
		opt(opts)
	}

	opts.limit = capListLimit(opts.limit, opts.logger)

	// Log index hint usage for monitoring and debugging
	logHintUsage(opts.logger, opts.hint, collection.Name(), opts.filter)

	// Build optimized pipeline with proper stage ordering
	pipeline := buildPipeline(opts)

	// Execute explain analysis if requested
	executeExplainIfRequested(ctx, collection, pipeline, opts.explain, opts.logger)

	cursor, err := collection.Aggregate(ctx, pipeline, nil)
	if err != nil {
		return nil, err
	}

	defer func() {
		if closeErr := cursor.Close(ctx); closeErr != nil {
			// Log the close error but don't override the main error
			// In production, this should be logged with proper logger
			_ = closeErr // silence linter
		}
	}()

	var result *ListResult[T]
	if cursor.Next(ctx) {
		if err = cursor.Decode(&result); err != nil {
			return nil, err
		}
	}

	if err = cursor.Err(); err != nil {
		return nil, err
	}

	// If no result was found, create an empty result structure
	if result == nil {
		result = &ListResult[T]{
			Items: []T{},
			Total: 0,
		}
	}

	if !opts.includeTotal {
		result.Total = -1
	}

	return result, nil
}
