// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"cmp"
	"context"
	"iter"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/altessa-s/go-atlas/core/types/ptr"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Cursor pagination constants
const (
	// DefaultCursorIdField is the default MongoDB field name used for cursor-based pagination.
	// This field must contain a MongoDB ObjectID (24 hexadecimal characters).
	DefaultCursorIdField = "cursor_id"
)

// Internal cursor pagination constants (implementation details)
const (
	sortDirectionAscending  = 1  // Ascending sort order
	sortDirectionDescending = -1 // Descending sort order

	// paginationLookaheadCount is the number of extra items fetched to determine if more pages exist.
	// We fetch limit+1 items to efficiently detect if there's a next page without a separate count query.
	paginationLookaheadCount = 1

	// ulidLength is the length of a ULID string (26 characters in Crockford's Base32).
	ulidLength = 26
)

// listCursorOptions holds configuration for cursor-based pagination queries.
// This struct contains all the parameters needed to execute an efficient cursor-based
// paginated query against a MongoDB collection. Cursor-based pagination provides O(1)
// performance regardless of page depth, unlike offset-based pagination.
//
// The options control filtering, sorting, projection, pagination limits, and query
// optimization features like index hints and explain analysis.
type listCursorOptions struct {
	sort             bson.D
	filter           bson.M
	projection       bson.M
	limit            int64
	cursor           *Cursor
	cursorToken      string // Raw cursor token from client (for storage mode)
	logger           *slog.Logger
	hint             any    // Index hint for query optimization
	explain          bool   // Enable query execution plan analysis
	cursorIdField    string // MongoDB field name for cursor ID (default: "cursor_id")
	includeTotal     bool
	storage          CursorStorage      // Server-side cursor storage (optional)
	collation        *options.Collation // Collation for string comparison rules
	stages           bson.A             // Custom pipeline stages inserted after $sort, before $facet
	decorationStages bson.A             // Custom pipeline stages inserted inside $facet → items, after $limit
}

// defaultListCursorOptions returns default configuration for cursor-based pagination.
// This function initializes sensible defaults that work for most use cases:
//   - Sort: DefaultListSort (created_at descending)
//   - Filter: Empty (no filtering)
//   - Limit: DefaultListLimit (100 items per page)
//   - Logger: DiscardLogger (no logging)
//   - includeTotal: true (compute total count)
//
// These defaults can be overridden using functional options like WithListCursorSort,
// WithListCursorFilter, WithListCursorLimit, etc.
func defaultListCursorOptions() *listCursorOptions {
	return &listCursorOptions{
		sort:          DefaultListSort,
		filter:        bson.M{},
		limit:         DefaultListLimit,
		cursor:        nil,
		logger:        discardLogger,
		cursorIdField: DefaultCursorIdField,
		includeTotal:  false,
	}
}

// ListCursorOption represents a functional option for configuring cursor-based list operations.
type ListCursorOption func(*listCursorOptions)

// WithListCursorLogger configures a custom logger for cursor-based list operations.
func WithListCursorLogger(logger *slog.Logger) ListCursorOption {
	return func(options *listCursorOptions) {
		if logger != nil {
			options.logger = logger
		}
	}
}

// WithListCursorCollation configures locale-aware collation for cursor-based list operations.
// This is useful for sorting string fields according to language-specific rules
// (e.g., Cyrillic alphabetical order with Russian locale).
//
// Example:
//
//	WithListCursorCollation(&options.Collation{Locale: "ru", Strength: 2})
func WithListCursorCollation(collation *options.Collation) ListCursorOption {
	return func(opts *listCursorOptions) {
		opts.collation = collation
	}
}

// WithListCursorSort configures the sort order for cursor-based list operations.
// It accepts string, *string, or bson.D types for maximum flexibility.
//
// Parameters:
//   - sort: Sort specification (string like "created_at,-updated_at", *string, or bson.D)
//
// String format:
//   - Field names separated by commas
//   - Prefix with "-" for descending order (e.g., "-created_at")
//
// Example:
//
//	WithListCursorSort("-created_at")
//	WithListCursorSort(bson.D{{"created_at", -1}, {"_id", -1}})
func WithListCursorSort[T ~string | ~*string | bson.D](sort T) ListCursorOption {
	return func(options *listCursorOptions) {
		if parsed, ok := parseSortOption(sort); ok {
			options.sort = parsed
		}
	}
}

// WithListCursorIdField specifies which field contains the cursor ID for tie-breaking.
// By default, this is "cursor_id", but you can configure it to use a different field
// (e.g., "_id" if your collection uses MongoDB ObjectID as the primary key).
//
// IMPORTANT: The specified field MUST contain a MongoDB ObjectID (24 hexadecimal characters).
//
// Parameters:
//   - cursorIdField: The MongoDB field name containing the ObjectID cursor ID
//
// Example:
//
//	WithListCursorIdField("_id")          // Use _id field as cursor ID
//	WithListCursorIdField("cursor_id")    // Use custom cursor_id field (default)
//	WithListCursorIdField("ulid")         // Use custom ulid field
func WithListCursorIdField(cursorIdField string) ListCursorOption {
	return func(options *listCursorOptions) {
		if cursorIdField != "" {
			options.cursorIdField = cursorIdField
		}
	}
}

// WithListCursorFilter configures the query filter for cursor-based list operations.
// The filter follows MongoDB query syntax for matching documents.
//
// Parameters:
//   - filter: MongoDB query filter using bson.M syntax
//
// Example:
//   - WithListCursorFilter(bson.M{"active": true})
//   - WithListCursorFilter(bson.M{"age": bson.M{"$gte": 18}})
func WithListCursorFilter(filter bson.M) ListCursorOption {
	return func(options *listCursorOptions) {
		options.filter = filter
	}
}

// WithListCursorProjection configures field projection for cursor-based list operations.
// This controls which fields are included or excluded from the results.
//
// Parameters:
//   - projection: MongoDB projection specification using bson.M syntax
//
// Example:
//   - WithListCursorProjection(bson.M{"name": 1, "email": 1}) // include only name and email
//   - WithListCursorProjection(bson.M{"password": 0}) // exclude password field
func WithListCursorProjection(projection bson.M) ListCursorOption {
	return func(options *listCursorOptions) {
		options.projection = projection
	}
}

// WithListCursorTotal controls whether the total count is computed.
// When disabled, the query skips the $count stage and ListCursorResult.Total is set to -1.
func WithListCursorTotal(include bool) ListCursorOption {
	return func(options *listCursorOptions) {
		options.includeTotal = include
	}
}

// WithListCursorLimit configures the page size for cursor-based pagination.
//
// Parameters:
//   - limit: Maximum number of items to return per page
//
// Example:
//
//	WithListCursorLimit(50)
func WithListCursorLimit(limit int64) ListCursorOption {
	return func(options *listCursorOptions) {
		if limit > 0 {
			options.limit = limit
		}
	}
}

// WithListCursorCursor sets the cursor for pagination.
// Use the NextCursor from the previous response to fetch the next page.
//
// The cursor can be provided as:
//   - string: ULID token (stateful mode) or base64-encoded JSON (stateless mode)
//   - *string: Pointer to string
//   - *Cursor: Direct cursor object (for backward compatibility)
//
// When using server-side storage, this should be the ULID token received from
// the previous page. The actual cursor metadata will be loaded from storage.
//
// Parameters:
//   - cursor: Cursor from previous page response
//
// Example:
//
//	WithListCursorCursor(previousResult.NextCursor)
func WithListCursorCursor[T interface{ ~string | ~*string | *Cursor }](cursor T) ListCursorOption {
	return func(options *listCursorOptions) {
		switch v := any(cursor).(type) {
		case string:
			options.cursorToken = v
		case *string:
			if v != nil {
				options.cursorToken = *v
			}
		case *Cursor:
			options.cursor = v
		}
	}
}

// WithListCursorHint configures an index hint for cursor-based list operations.
//
// Parameters:
//   - hint: Index hint (string name, bson.D, or bson.M)
//
// Example:
//
//	WithListCursorHint("created_at_1__id_1")
//	WithListCursorHint(bson.D{{"created_at", 1}, {"_id", 1}})
func WithListCursorHint(hint any) ListCursorOption {
	return func(options *listCursorOptions) {
		options.hint = hint
	}
}

// WithListCursorExplain enables query execution plan analysis.
// Requires a logger to be configured with WithListCursorLogger.
//
// Example:
//
//	WithListCursorExplain()
func WithListCursorExplain() ListCursorOption {
	return func(options *listCursorOptions) {
		options.explain = true
	}
}

// WithListCursorStorage configures server-side cursor storage.
// When storage is provided, cursor metadata is stored on the server instead of being
// encoded in the cursor token sent to the client. This provides several benefits:
//   - Security: Clients cannot tamper with cursor metadata
//   - Smaller tokens: Client receives only a storage key (ULID, ~26 chars)
//   - Flexibility: Storage can implement TTL, rate limiting, audit logging
//
// When storage is NOT provided (default), cursor metadata is base64-encoded and sent
// to the client (stateless mode). This mode works without external dependencies
// but has larger tokens and no server-side control.
//
// Example with memory storage:
//
//	storage := memory.New(1 * time.Hour)
//	defer storage.Close()
//	result, err := mongotools.ListCursor(ctx, collection,
//	    mongotools.WithListCursorStorage(storage),
//	    mongotools.WithListCursorLimit(50),
//	)
func WithListCursorStorage(storage CursorStorage) ListCursorOption {
	return func(options *listCursorOptions) {
		options.storage = storage
	}
}

// WithListCursorStages adds custom aggregation pipeline stages that are inserted after $sort
// and before $facet. These stages operate on the entire filtered and sorted result set.
//
// Typical use cases include $lookup for joining data needed for sorting or filtering,
// $addFields for computed fields, or $unwind for denormalization.
//
// Multiple calls accumulate stages in order. Each bson.D represents one pipeline stage.
//
// WARNING: Do not modify the cursor ID field or sort fields in these stages, as this
// will break cursor-based pagination.
//
// When WithListCursorTotal(true) is used together with custom stages, the total count is
// pre-computed via $setWindowFields (requires MongoDB 5.0+) BEFORE stages are applied.
// This ensures the count reflects the original document cardinality even when stages
// contain $unwind or other cardinality-changing operations.
//
// Example:
//
//	WithListCursorStages(
//	    bson.D{{"$lookup", bson.M{
//	        "from":         "categories",
//	        "localField":   "category_id",
//	        "foreignField": "_id",
//	        "as":           "category",
//	    }}},
//	    bson.D{{"$unwind", "$category"}},
//	)
func WithListCursorStages(stages ...bson.D) ListCursorOption {
	return func(opts *listCursorOptions) {
		for _, stage := range stages {
			if stage != nil {
				opts.stages = append(opts.stages, stage)
			}
		}
	}
}

// WithListCursorDecorationStages adds custom aggregation pipeline stages that are inserted
// inside the $facet → items branch, after $limit. These stages operate only on the current
// page of results (limit+1 documents), making them efficient for expensive operations.
//
// Typical use cases include $lookup for data needed only for display (e.g., translations,
// user profiles), $addFields for presentation-layer computed fields.
//
// Multiple calls accumulate stages in order. Each bson.D represents one pipeline stage.
//
// WARNING: Do not modify the cursor ID field or sort fields in these stages, as this
// will break cursor-based pagination.
//
// Example:
//
//	WithListCursorDecorationStages(
//	    bson.D{{"$lookup", bson.M{
//	        "from":         "translations",
//	        "localField":   "_id",
//	        "foreignField": "entity_id",
//	        "as":           "translations",
//	    }}},
//	)
func WithListCursorDecorationStages(stages ...bson.D) ListCursorOption {
	return func(opts *listCursorOptions) {
		for _, stage := range stages {
			if stage != nil {
				opts.decorationStages = append(opts.decorationStages, stage)
			}
		}
	}
}

// ListCursorResult represents the result of a cursor-based paginated list operation.
type ListCursorResult[T any] struct {
	// Items contains the actual data items returned from the query
	Items []T `bson:"items"`

	// Total contains the total number of items available (before pagination).
	// A value of -1 indicates the total was not computed.
	Total int64 `bson:"total"`

	// NextCursor points to the next page. Nil if this is the last page.
	NextCursor *string `bson:"next_cursor,omitempty"`
}

// ListCursor performs a cursor-based paginated query on the collection.
// Returns items with total count and next cursor for subsequent pages.
//
// Cursor-based pagination provides O(1) performance regardless of page depth,
// unlike offset-based pagination which degrades to O(n) for large offsets.
//
// Cursor Validation:
// All cursors must include metadata (sort field, direction, cursor ID field) and a
// SHA-256 checksum of critical parameters. ListCursor validates that pagination
// parameters haven't changed between requests, preventing incorrect pagination
// results and security issues.
//
// If validation fails or the cursor is missing required metadata, an error is returned.
//
// Requirements:
//   - Collection must have a composite index on (sort_field, _id)
//   - Sort field must contain timestamp in Unix milliseconds
//   - Documents must have _id field (UUID string)
//   - Pagination parameters (sort, cursorIdField) must remain consistent between requests
//
// Index Example:
//
//	db.collection.createIndex({"created_at": -1, "_id": -1})
//
// Basic Example:
//
//	type User struct {
//	    Id        string `bson:"_id"`
//	    Name      string `bson:"name"`
//	    CreatedAt int64  `bson:"created_at"`
//	}
//
//	// First page
//	result, err := ListCursor[User](ctx, collection,
//	    WithListCursorLimit(10),
//	    WithListCursorSort("-created_at"),
//	)
//
//	// Next page - cursor automatically validates parameters
//	if result.NextCursor != nil {
//	    nextResult, err := ListCursor[User](ctx, collection,
//	        WithListCursorLimit(10),
//	        WithListCursorSort("-created_at"),
//	        WithListCursorCursor(result.NextCursor),
//	    )
//	    // Returns error if sort or cursorIdField changed
//	}
//
// With Filtering:
//
//	result, err := ListCursor[User](ctx, collection,
//	    WithListCursorFilter(bson.M{"status": "active"}),
//	    WithListCursorLimit(20),
//	    WithListCursorSort("-created_at"),
//	    WithListCursorHint("status_1_created_at_-1__id_-1"),
//	)
func ListCursor[T any](ctx context.Context, collection *mongo.Collection, o ...ListCursorOption) (*ListCursorResult[T], error) {
	opts := defaultListCursorOptions()
	for _, opt := range o {
		opt(opts)
	}

	opts.limit = capListLimit(opts.limit, opts.logger)

	// Parse cursor token if provided (handles both ULID and stateless formats)
	if opts.cursorToken != "" {
		cursor, err := parseCursorToken(ctx, opts.cursorToken, opts.storage, opts.filter)
		if err != nil {
			return nil, err
		}
		opts.cursor = cursor
	}

	// If cursor is provided, validate and use its metadata
	// This ensures pagination parameters remain consistent between requests
	if opts.cursor != nil && !opts.cursor.IsZero() {
		// For stateless cursors (no storage), validate checksum
		if opts.storage == nil {
			if err := opts.cursor.ValidateChecksum(opts.sort, opts.cursorIdField); err != nil {
				return nil, coreerrs.WrapOperation(err, "validate cursor")
			}
		}

		// Override options with cursor metadata to ensure consistency
		// This prevents using different sort/cursor fields between requests
		cursorSort, err := opts.cursor.GetSort()
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "get sort from cursor")
		}
		opts.sort = cursorSort

		// Use cursor's ID field, or default if empty (optimization for smaller cursors)
		opts.cursorIdField = cmp.Or(opts.cursor.CursorIdField, DefaultCursorIdField)
	}

	// Log index hint usage
	logHintUsage(opts.logger, opts.hint, collection.Name(), opts.filter,
		"cursor_provided", opts.cursor != nil)

	// Build optimized pipeline
	pipeline := buildCursorPipeline(opts, collection.Name())

	// Execute explain analysis if requested
	executeExplainIfRequested(ctx, collection, pipeline, opts.hint, opts.explain, opts.logger)

	aggOpts := options.Aggregate()
	if opts.hint != nil {
		aggOpts.SetHint(opts.hint)
	}
	if opts.collation != nil {
		aggOpts.SetCollation(opts.collation)
	}

	cursor, err := collection.Aggregate(ctx, pipeline, aggOpts)
	if err != nil {
		return nil, err
	}

	defer func() { //nolint:contextcheck // intentionally uses background context for cleanup after cancellation
		// Use a fresh context for cleanup to avoid issues when original context is canceled.
		// This ensures cursor is always properly closed even during error/cancellation paths.
		const cursorCloseTimeout = 5 * time.Second
		closeCtx, cancel := context.WithTimeout(context.Background(), cursorCloseTimeout)
		defer cancel()
		if closeErr := cursor.Close(closeCtx); closeErr != nil {
			// Log the close error but don't override the main error
			opts.logger.WarnContext(closeCtx, "failed to close MongoDB cursor",
				"error", closeErr,
				"collection", collection.Name(),
			)
		}
	}()

	var pipelineResult struct {
		Items []T   `bson:"items"`
		Total int64 `bson:"total"`
	}

	if cursor.Next(ctx) {
		if err = cursor.Decode(&pipelineResult); err != nil {
			return nil, err
		}
	}

	if err = cursor.Err(); err != nil {
		return nil, err
	}

	result := &ListCursorResult[T]{
		Items:      []T{},
		Total:      pipelineResult.Total,
		NextCursor: nil,
	}

	// Check if we have more pages
	// We fetched limit+1 items, so if we got more than limit, there's a next page
	hasMore := int64(len(pipelineResult.Items)) > opts.limit

	if hasMore {
		// Remove the extra item (limit+1)
		result.Items = pipelineResult.Items[:opts.limit]

		// Create a cursor from the last item
		lastItem := result.Items[len(result.Items)-1]

		// Extract cursor ID and sort field value from last item
		cursorId, sortValue, err := extractCursorDataFromItem(lastItem, opts.cursorIdField, opts.sort)
		if err != nil {
			// Cursor build failure indicates data inconsistency (e.g., invalid ID format)
			// Return error instead of silently continuing without pagination
			return nil, coreerrs.WrapOperation(err, "extract cursor id")
		}

		// Generate next cursor token (ULID if storage configured, base64 JSON otherwise)
		nextCursorToken, err := generateNextCursorToken(ctx, cursorId, opts.sort, opts.cursorIdField, opts.filter, opts.storage, sortValue)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "generate next cursor")
		}
		result.NextCursor = ptr.Wrap(nextCursorToken)
	} else {
		// No more pages, return all items
		result.Items = pipelineResult.Items
	}

	if !opts.includeTotal {
		result.Total = -1
	}

	return result, nil
}

// ListCursorSeq provides an idiomatic Go 1.25 iterator over all items in a paginated result set.
// It automatically handles fetching subsequent pages using next cursors until the end of the collection.
//
// The iterator yields (item, error) pairs. If an error occurs during pagination (e.g., network failure,
// invalid cursor), the error is yielded and iteration stops.
//
// Example:
//
//	for user, err := range mongotools.ListCursorSeq[User](ctx, coll, mongotools.WithListCursorLimit(100)) {
//	    if err != nil {
//	        return err
//	    }
//	    fmt.Println(user.Name)
//	}
func ListCursorSeq[T any](ctx context.Context, collection *mongo.Collection, o ...ListCursorOption) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		// Pre-allocate options slice with room for the cursor option.
		// On subsequent pages append reuses the backing array (cap = len+1).
		pageOpts := make([]ListCursorOption, len(o), len(o)+1)
		copy(pageOpts, o)

		for {
			result, err := ListCursor[T](ctx, collection, pageOpts...)
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}

			// Yield all items in the current page
			for _, item := range result.Items {
				if !yield(item, nil) {
					return
				}
			}

			// Check if we reached the end
			if result.NextCursor == nil {
				return
			}

			// Set cursor for next page, reusing the pre-allocated slice.
			pageOpts = append(pageOpts[:len(o)], WithListCursorCursor(*result.NextCursor))
		}
	}
}
