// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"cmp"
	"context"
	"iter"
	"log/slog"
	"reflect"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas/store/mongo/internal/query"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
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
	// codec is selected after all options are applied and shared by every page.
	codec cursorCodec `opt:"-"`
	// WithListCursorSort parses string forms and ignores invalid sorts.
	sort bson.D `optgen:"manual"`
	// WithListCursorFilter configures the query filter for cursor-based list operations.
	// The filter follows MongoDB query syntax for matching documents.
	//
	// Parameters:
	//   - filter: bson.D, bson.M, map[string]any, bson.Raw, or nil
	//
	// Example:
	//   - WithListCursorFilter(bson.M{"active": true})
	//   - WithListCursorFilter(bson.M{"age": bson.M{"$gte": 18}})
	filter any `optgen:"allownull"`
	// WithListCursorFilterEncoding overrides inherited filter encoding settings.
	filterEncoding FilterEncoding
	// prepared is the validated filter sent to MongoDB.
	prepared bson.Raw `opt:"-"`
	// WithListCursorProjection configures field projection for cursor-based list operations.
	// This controls which fields are included or excluded from the results.
	//
	// Parameters:
	//   - projection: MongoDB projection specification using bson.M syntax
	//
	// The next-page cursor is read from the last returned item, so the projection
	// must return the cursor ID field and the primary sort field in full and as
	// stored — with a 1/true flag on the field or an ancestor, never an
	// expression; [ListCursor] rejects one that does not with
	// [ErrProjectionDropsCursorField]. Use dotted paths: the nested-document form
	// ({a: {b: 1}}) is not recognized and is rejected for cursor fields.
	// An empty projection adds no $project stage.
	//
	// Example:
	//   - WithListCursorProjection(bson.M{"name": 1, "email": 1}) // include only name and email (_id stays)
	//   - WithListCursorProjection(bson.M{"password": 0}) // exclude password field
	projection bson.M `optgen:"allownull"`
	// WithListCursorLimit configures the page size for cursor-based pagination.
	//
	// Parameters:
	//   - limit: Maximum number of items to return per page
	//
	// Example:
	//
	//	WithListCursorLimit(50)
	limit int64 `optcheck:"min=1" optgen:"onfail=skip"`
	// WithListCursorCursor parses token forms into cursor or cursorToken.
	cursor *Cursor `optgen:"manual"`
	// Set by WithListCursorCursor when given string forms.
	cursorToken string `optgen:"manual"`
	// WithListCursorLogger configures a custom logger for cursor-based list operations.
	logger *slog.Logger `optgen:"notnil"`
	// WithListCursorHint configures an index hint for cursor-based list operations.
	//
	// Parameters:
	//   - hint: Index hint (string name, bson.D, or bson.M)
	//
	// Example:
	//
	//	WithListCursorHint("created_at_1__id_1")
	//	WithListCursorHint(bson.D{{"created_at", 1}, {"_id", 1}})
	hint any `optgen:"allownull"` // Index hint for query optimization
	// WithListCursorExplain enables query execution plan analysis.
	// Requires a logger to be configured with WithListCursorLogger.
	//
	// Example:
	//
	//	WithListCursorExplain()
	explain bool `optgen:"default=false"`
	// WithListCursorIdField ignores empty strings.
	cursorIdField string `optgen:"manual,default=DefaultCursorIdField"`
	// WithListCursorTotal controls whether the total count is computed.
	// When disabled, the query skips the $count stage and ListCursorResult.Total is set to -1.
	includeTotal bool `opt:"Total" optval:"param"`
	// WithListCursorStorage configures server-side cursor storage.
	// When storage is provided, cursor metadata is stored on the server instead of being
	// encoded in the cursor token sent to the client. This provides several benefits:
	//   - Security: Clients cannot tamper with cursor metadata
	//   - Smaller tokens: Client receives only a storage key (ULID, ~26 chars)
	//   - Flexibility: Storage can implement TTL, rate limiting, audit logging
	//
	// With storage configured, only storage keys are accepted as cursors: a
	// stateless token or a direct *Cursor fails with [ErrInvalidCursorFormat].
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
	storage Storage `optgen:"allownull"` // Server-side cursor storage (optional)
	// WithListCursorSigner makes stateless cursor tokens signed. The token then
	// carries, under the signer's signature, the collection it was issued for
	// and the subject (WithListCursorSubject) next to the pagination metadata,
	// and a continuation is held to both exactly as in stateful mode
	// ([ErrCursorCollectionMismatch], [ErrCursorSubjectMismatch]). A token the
	// signer does not verify — tampered, expired, issued under another key, or
	// an unsigned base64 token — fails with [ErrInvalidCursor], and a direct
	// *Cursor with [ErrInvalidCursorFormat].
	//
	// The signer is a go-aip pagetoken codec (HMAC-SHA256, key rotation, TTL,
	// optional encryption), the one keyset.FindPage takes; build it with
	// pagetoken.New or factory/ext/aip/pagination/pagetoken. It has no effect
	// together with WithListCursorStorage: stored cursors need no signature.
	// A nil signer is ignored.
	//
	// Example:
	//
	//	codec, err := pagetoken.New(signingKey)
	//	result, err := mongotools.ListCursor(ctx, collection,
	//	    mongotools.WithListCursorSigner(codec),
	//	    mongotools.WithListCursorSubject(tenantID),
	//	)
	signer CursorSigner `optgen:"notnil"`
	// WithListCursorCollation configures locale-aware collation for cursor-based list operations.
	// This is useful for sorting string fields according to language-specific rules
	// (e.g., Cyrillic alphabetical order with Russian locale).
	//
	// Example:
	//
	//	WithListCursorCollation(&options.Collation{Locale: "ru", Strength: 2})
	collation *options.Collation `optgen:"allownull"` // Collation for string comparison rules
	// WithListCursorStages normalizes stages into bson.A.
	stages bson.A `optgen:"manual"`
	// WithListCursorDecorationStages normalizes stages into bson.A.
	decorationStages bson.A `optgen:"manual"`
	// WithListCursorSubject ignores empty strings.
	subject string `optgen:"manual"`
}

// defaultListCursorOptions returns default configuration for cursor-based pagination.
// This function initializes sensible defaults that work for most use cases:
//   - Sort: DefaultListSort (created_at descending)
//   - Filter: Empty (no filtering)
//   - Limit: DefaultListLimit (100 items per page)
//   - Logger: DiscardLogger (no logging)
//   - includeTotal: false (skip total count)
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

func newListCursorOptions(options ...ListCursorOption) *listCursorOptions {
	opts := defaultListCursorOptions()
	for _, opt := range options {
		opt(opts)
	}
	opts.codec = selectCursorCodec(opts)
	return opts
}

// ListCursorOption represents a functional option for configuring cursor-based list operations.
type ListCursorOption func(*listCursorOptions)

// WithListCursorSort configures the sort order for cursor-based list operations.
// It accepts string, *string, or bson.D types for maximum flexibility.
//
// Parameters:
//   - sort: Sort specification (string like "-created_at", *string, or bson.D)
//
// String format:
//   - Field names separated by commas
//   - Prefix with "-" for descending order (e.g., "-created_at")
//
// A cursor resumes on a single sort value, so [ListCursor] accepts one sort
// field, optionally followed by the cursor-ID field in the same direction, or
// a sort led by the cursor-ID field; any other shape fails with
// [ErrUnsupportedCursorSort].
//
// Example:
//
//	WithListCursorSort("-created_at")
//	WithListCursorSort(bson.D{{"created_at", -1}, {"cursor_id", -1}})
func WithListCursorSort[T ~string | ~*string | bson.D](sort T) ListCursorOption {
	return func(options *listCursorOptions) {
		if parsed, ok := query.ParseSortOption(sort); ok {
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

// WithListCursorCursor sets the cursor for pagination.
// Use the NextCursor from the previous response to fetch the next page.
//
// The cursor can be provided as:
//   - string: ULID token (stateful mode) or base64-encoded JSON (stateless mode)
//   - *string: Pointer to string
//   - *Cursor: Direct cursor object
//
// When using server-side storage, this must be the ULID token received from
// the previous page; the actual cursor metadata will be loaded from storage.
// A stateless token or a *Cursor is then rejected with [ErrInvalidCursorFormat].
//
// Parameters:
//   - cursor: Cursor from previous page response
//
// Example:
//
//	WithListCursorCursor(previousResult.NextCursor)
func WithListCursorCursor[T interface{ ~string | ~*string | *Cursor }](cursor T) ListCursorOption {
	return func(options *listCursorOptions) {
		if c, ok := any(cursor).(*Cursor); ok {
			options.cursor = c
			return
		}
		value := reflect.Indirect(reflect.ValueOf(cursor))
		if value.IsValid() {
			options.cursorToken = value.String()
		}
	}
}

// WithListCursorSubject binds cursors to the requesting principal
// (typically a tenant or user id extracted from the request context). When set,
// the "next" token records the subject — in the stored metadata, or under the
// signature of a signed token — and a continuation
// request whose subject differs is rejected with [ErrCursorSubjectMismatch] —
// so a cursor token leaked or guessed by another principal cannot be
// replayed to page through the original principal's data. The match is exact:
// a cursor minted without a subject is rejected by a request that passes one.
//
// It requires WithListCursorStorage (stateful mode) or WithListCursorSigner
// (signed stateless tokens). An unsigned stateless token cannot carry a
// subject the client could not rewrite, so ListCursor given a subject and
// neither of the two fails with [ErrCursorSubjectNotEnforced]. An empty subject
// leaves cursors unbound, which is the default; an unbound cursor continues
// only under an empty subject.
//
//	p, _ := principal.FromContext(ctx)
//	res, err := mongo.ListCursor[User](ctx, coll,
//		mongo.WithListCursorStorage(store),
//		mongo.WithListCursorSubject(p.Tenant),
//	)
func WithListCursorSubject(subject string) ListCursorOption {
	return func(options *listCursorOptions) {
		options.subject = subject
	}
}

// WithListCursorStages adds custom aggregation pipeline stages that are inserted after $sort
// and before the items branch. These stages operate on the entire filtered and sorted result
// set fed into the items branch.
//
// Typical use cases include $lookup for joining data needed for sorting or filtering,
// $addFields for computed fields, or $unwind for denormalization.
//
// Multiple calls accumulate stages in order. Each bson.D represents one pipeline stage.
//
// WARNING: Do not modify the cursor ID field or sort fields in these stages, as this
// will break cursor-based pagination.
//
// Effect on WithListCursorTotal:
//
// Total is always computed by an independent count sub-pipeline (via $unionWith, requires
// MongoDB 4.4+) scoped to the user filter only. Custom stages added here DO NOT run inside
// the count sub-pipeline — total reflects the source-document cardinality, not whatever
// cardinality the stages would produce. This holds for both shrinking ($match on joined
// data) and expanding ($unwind) stages.
//
// Example: with WithListCursorStages($lookup-categories, $match: category.archived=false)
// the items branch returns only documents whose joined category is not archived, but total
// counts ALL documents matching the user filter — the joined-data $match does not filter
// the count. If you need total to reflect post-stage cardinality, push the equivalent
// predicate into the user filter (filter.WithMatch / repo-level filter) instead of into
// stages.
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
			opts.stages = slices.AppendNonNil[any](opts.stages, stage)
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
			opts.decorationStages = slices.AppendNonNil[any](opts.decorationStages, stage)
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
//   - Pagination parameters (sort, cursorIdField) must remain consistent between requests,
//     in stateless and in stateful mode alike: a continuation that omits the sort requests
//     the default one, and fails with [ErrCursorChecksumMismatch] if the cursor has another
//   - A stateful cursor is bound to its collection: presenting it to a ListCursor call on
//     another one fails with [ErrCursorCollectionMismatch]
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
	opts, err := prepareListCursorOptions(ctx, collection, o)
	if err != nil {
		return nil, err
	}
	return listCursorPage[T](ctx, collection, opts)
}

func prepareListCursorOptions(ctx context.Context, collection *mongo.Collection, o []ListCursorOption) (*listCursorOptions, error) {
	opts := newListCursorOptions(o...)
	raw, err := prepareFilter(ctx, collection, opts.filter, opts.filterEncoding)
	if err != nil {
		return nil, err
	}
	opts.prepared = raw
	return opts, nil
}

// Each page resolves its own continuation against the same prepared filter.
func listCursorPage[T any](ctx context.Context, collection *mongo.Collection, opts *listCursorOptions) (*ListCursorResult[T], error) {
	queryCtx, cancel := corecontext.WithMaxTimeout(ctx, DefaultQueryTimeout)
	defer cancel()

	opts.limit = capListLimit(opts.limit, opts.logger)

	collectionDigest := cursorCollectionDigest(collection)
	if err := resolveCursorOptions(queryCtx, opts, collectionDigest); err != nil {
		return nil, err
	}

	// Log index hint usage
	logHintUsage(opts.logger, opts.hint, collection.Name(), opts.prepared,
		"cursor_provided", opts.cursor != nil)

	// Build optimized pipeline
	pipeline := buildCursorPipeline(opts, collection.Name())

	// Execute explain analysis if requested
	executeExplainIfRequested(queryCtx, collection, pipeline, opts.hint, opts.explain, opts.logger)

	aggOpts := options.Aggregate()
	if opts.hint != nil {
		aggOpts.SetHint(opts.hint)
	}
	if opts.collation != nil {
		aggOpts.SetCollation(opts.collation)
	}

	cursor, err := collection.Aggregate(queryCtx, pipeline, aggOpts)
	if err != nil {
		return nil, err
	}

	defer func() { //nolint:contextcheck // uses a detached bounded context for cleanup after cancellation
		// Use a fresh context for cleanup to avoid issues when original context is canceled.
		// This ensures cursor is always properly closed even during error/cancellation paths.
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(queryCtx), DefaultCursorCloseTimeout)
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

	if cursor.Next(queryCtx) {
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

		// Generate next cursor token (ULID if storage configured, signed if a signer is, base64 JSON otherwise)
		nextCursorToken, err := opts.nextCursorToken(ctx, cursorId, sortValue, collectionDigest)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "generate next cursor")
		}
		result.NextCursor = new(nextCursorToken)
	} else {
		// No more pages, return all items
		result.Items = pipelineResult.Items
	}

	if !opts.includeTotal {
		result.Total = -1
	}

	return result, nil
}

// resolveCursorOptions loads the cursor the options carry (a storage key or
// a stateless token), validates it, adopts its sort and cursor ID field so
// pagination parameters stay consistent between requests, and validates the
// resulting sort and projection. collection is the digest a stateful cursor
// is bound to, see [cursorCollectionDigest].
func resolveCursorOptions(ctx context.Context, opts *listCursorOptions, collection string) error {
	cursor, err := opts.codec.resolve(ctx, opts, collection)
	if err != nil {
		return err
	}
	opts.cursor = cursor
	if !cursor.IsZero() {
		// Override options with cursor metadata to ensure consistency
		// This prevents using different sort/cursor fields between requests
		cursorSort, err := opts.cursor.GetSort()
		if err != nil {
			return coreerrs.WrapOperation(err, "get sort from cursor")
		}
		opts.sort = cursorSort

		// Use cursor's ID field, or default if empty (optimization for smaller cursors)
		opts.cursorIdField = cmp.Or(opts.cursor.CursorIdField, DefaultCursorIdField)

		if err := validateCursorSortValue(opts.cursor, opts.sort, opts.cursorIdField); err != nil {
			return coreerrs.WrapOperation(err, "validate cursor")
		}
	}

	if err := validateCursorSort(opts.sort, opts.cursorIdField); err != nil {
		return err
	}
	if err := validateCursorProjection(opts.projection, opts.cursorIdField, opts.sort); err != nil {
		return err
	}
	return nil
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
		opts, err := prepareListCursorOptions(ctx, collection, o)
		if err != nil {
			var zero T
			yield(zero, err)
			return
		}
		// Each continuation validates against the caller's original sort and ID
		// field. Reuse the request allocation for the page the codec receives.
		initial := *opts
		for {
			result, err := listCursorPage[T](ctx, collection, opts)
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

			*opts = initial
			opts.cursor = nil
			opts.cursorToken = *result.NextCursor
		}
	}
}

func (o *listCursorOptions) binding() cursorFilter {
	return cursorFilter{raw: o.prepared, original: o.filter}
}
