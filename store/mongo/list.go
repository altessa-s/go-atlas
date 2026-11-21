// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/altessa-s/go-atlas/store/mongo/internal/pipeline"
	"github.com/altessa-s/go-atlas/store/mongo/internal/query"

	corecontext "github.com/altessa-s/go-atlas-core/context"
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
	// MaxListOffset caps the offset (`$skip`) accepted by [List]. `$skip` is
	// O(N) — MongoDB has to read and discard every skipped document — so a
	// caller asking for offset=10_000_000 trivially overloads the server.
	// Beyond this cap the value is clamped (with a warning); use
	// [ListCursor] for deeper pagination.
	MaxListOffset = 10_000
)

// DefaultListSort is the default sort order for list operations
var DefaultListSort = bson.D{{Key: "created_at", Value: -1}}

// listOptions holds configuration for offset-based pagination queries.
// This struct contains all parameters needed to execute paginated queries using
// the traditional offset/limit approach. While simpler than cursor-based pagination,
// offset-based pagination has O(n) performance characteristics that degrade for deep pages.
//
// The options control filtering, sorting, projection, pagination boundaries, and query
// optimization features including index hints and performance analysis via explain.
type listOptions struct {
	// WithListSort parses string forms and ignores invalid sorts.
	sort bson.D `optgen:"manual"`
	// WithListFilter configures the query filter for list operations.
	// The filter follows MongoDB query syntax for matching documents.
	//
	// Parameters:
	//   - filter: bson.D, bson.M, map[string]any, bson.Raw, or nil
	//
	// Example:
	//   - WithListFilter(bson.M{"active": true})
	//   - WithListFilter(bson.M{"age": bson.M{"$gte": 18}})
	filter any `optgen:"allownull"`
	// WithListFilterEncoding overrides inherited filter encoding settings.
	filterEncoding FilterEncoding
	// prepared is the validated filter sent to MongoDB.
	prepared bson.Raw `opt:"-"`
	// WithListProjection configures field projection for list operations.
	// This controls which fields are included or excluded from the results.
	//
	// Parameters:
	//   - projection: MongoDB projection specification using bson.M syntax
	//
	// Example:
	//   - WithListProjection(bson.M{"name": 1, "email": 1}) // include only name and email
	//   - WithListProjection(bson.M{"password": 0}) // exclude password field
	projection bson.M `optgen:"allownull"`
	// WithListLimit updates limit and offset together and clamps both values.
	limit int64 `optgen:"manual"`
	// Set by WithListLimit together with limit.
	offset int64 `optgen:"manual"`
	// WithListLogger configures a custom logger for list operations.
	// This is useful for monitoring and debugging query performance.
	//
	// Parameters:
	//   - logger: Custom structured logger instance
	//
	// Example:
	//   - WithLogger(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	logger *slog.Logger `optgen:"notnil"`
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
	hint any `optgen:"allownull"` // Index hint for query optimization
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
	explain bool
	// WithListTotal controls whether the total count is computed.
	// Disabling the total avoids running an additional $count stage, which can dramatically
	// improve performance on large collections when the caller does not need the total.
	//
	// Parameters:
	//   - include: Set to false to skip total computation (ListResult.Total will be -1)
	includeTotal bool `opt:"Total" optval:"param"`
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
		logger:       discardLogger,
		includeTotal: true,
	}
}

// ListOption represents a functional option for configuring list operations.
type ListOption func(*listOptions)

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
		if parsed, ok := query.ParseSortOption(sort); ok {
			options.sort = parsed
		}
	}
}

// WithListLimit configures pagination for list operations.
// It sets both the maximum number of items to return and the offset.
//
// Both values are bounded:
//   - limit is capped at [MaxListLimit] (default 1000) to keep response sizes sane.
//   - offset is capped at [MaxListOffset] (default 10000) because `$skip` is O(N) on
//     the server side; deep offsets are a cheap way to overload MongoDB. For deeper
//     pagination use [ListCursor], which is O(1) per page.
//
// Parameters:
//   - limit: Maximum number of items to return (must be > 0, capped at [MaxListLimit])
//   - offset: Number of items to skip (must be >= 0, capped at [MaxListOffset])
//
// Example:
//   - WithListLimit(20, 0) // First page: items 1-20
//   - WithListLimit(20, 20) // Second page: items 21-40
//   - WithListLimit(2000, 0) // Automatically capped to MaxListLimit (1000)
//   - WithListLimit(20, 1_000_000) // Automatically capped to MaxListOffset (10000)
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

// ListResult represents the result of a paginated list operation.
// It contains both the items and the total count.
type ListResult[T any] struct {
	// Items contains the actual data items returned from the query
	Items []T `bson:"items" json:"items"`
	// Total contains the total number of items available (before pagination).
	// A value of -1 indicates the total was not computed.
	Total int64 `bson:"total" json:"total"`
}

// buildPipeline applies the effective default sort before rendering the offset query.
func buildPipeline(opts *listOptions) bson.A {
	sort := opts.sort
	if len(sort) == 0 {
		sort = DefaultListSort
	}
	return pipeline.Offset(opts.prepared, sort, opts.projection, opts.limit, opts.offset, opts.includeTotal)
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
	raw, err := prepareFilter(ctx, collection, opts.filter, opts.filterEncoding)
	if err != nil {
		return nil, err
	}
	opts.prepared = raw
	queryCtx, cancel := corecontext.WithMaxTimeout(ctx, DefaultQueryTimeout)
	defer cancel()

	opts.limit = capListLimit(opts.limit, opts.logger)
	opts.offset = capListOffset(opts.offset, opts.logger)

	// Log index hint usage for monitoring and debugging
	logHintUsage(opts.logger, opts.hint, collection.Name(), opts.prepared)

	// Build optimized pipeline with proper stage ordering
	pipeline := buildPipeline(opts)

	// Execute explain analysis if requested
	executeExplainIfRequested(queryCtx, collection, pipeline, opts.hint, opts.explain, opts.logger)

	aggOpts := options.Aggregate()
	if opts.hint != nil {
		aggOpts.SetHint(opts.hint)
	}
	cursor, err := collection.Aggregate(queryCtx, pipeline, aggOpts)
	if err != nil {
		return nil, err
	}

	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(queryCtx), DefaultCursorCloseTimeout)
		defer cancel()
		if closeErr := cursor.Close(closeCtx); closeErr != nil {
			// Log the close error but don't override the main error
			// In production, this should be logged with proper logger
			_ = closeErr // silence linter
		}
	}()

	var result *ListResult[T]
	if cursor.Next(queryCtx) {
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
