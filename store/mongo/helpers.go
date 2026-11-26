// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"fmt"
	"log/slog"
	"math/bits"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/altessa-s/go-mongox/bsonfilter"

	"github.com/altessa-s/go-atlas/store/mongo/internal/query"

	corehash "github.com/altessa-s/go-atlas-core/encoding/hash"
)

// discardLogger is a singleton logger that discards all output.
// Reused across defaultListOptions/defaultListCursorOptions to avoid
// allocating a new *slog.Logger per List/ListCursor call.
var discardLogger = slog.New(slog.DiscardHandler)

// BuildFilter capacity optimization constants (internal implementation details)
const (
	minimalCapacity             = 1
	pairsPerKeyValue            = 2
	capacityOverheadNumerator   = 5 // 125% = 5/4
	capacityOverheadDenominator = 4 // 125% = 5/4
	capacityRoundingAdjustment  = 3
)

// Array/slice indexing constants (internal implementation details)
const (
	firstElementIndex  = 0
	secondElementIndex = 1
)

// Query and aggregation operators the package's filters and pipelines reuse.
const (
	opAnd     = "$and"
	opEq      = "$eq"
	opMatch   = "$match"
	opProject = "$project"
	opCount   = "$count"
)

// BuildFilter creates a MongoDB filter with optimized memory allocation.
// It automatically calculates optimal capacity: pairs count + 25% + rounded to next power of 2.
// This reduces map reallocations while avoiding excessive memory waste.
//
// Parameters:
//   - kv: Key-value pairs as alternating strings and any values (must be even number of elements)
//
// Returns:
//   - bson.M: MongoDB filter with pre-allocated capacity
func BuildFilter(kv ...any) bson.M {
	pairCount := len(kv) / pairsPerKeyValue
	if pairCount == firstElementIndex {
		return bson.M{}
	}

	// Calculate optimal capacity: pairs + 25% overhead, rounded to the next power of 2
	targetCapacity := (pairCount*capacityOverheadNumerator + capacityRoundingAdjustment) / capacityOverheadDenominator
	optimalCapacity := nextPowerOfTwo(max(targetCapacity, minimalCapacity))

	f := make(bson.M, optimalCapacity)
	for i := range pairCount {
		keyIndex := i * pairsPerKeyValue
		valueIndex := keyIndex + secondElementIndex
		key, ok := kv[keyIndex].(string)
		if !ok {
			// Keep BuildFilter non-panicking; callers sometimes build filters dynamically.
			// Fall back to fmt.Sprint(...) for non-string keys.
			key = fmt.Sprint(kv[keyIndex])
		}
		f[key] = kv[valueIndex]
	}
	return f
}

const (
	// DefaultWriteTimeout is the timeout for write operations (insert, update, delete).
	DefaultWriteTimeout = 10 * time.Second
	// DefaultQueryTimeout caps read operations, including find and aggregation queries.
	DefaultQueryTimeout = query.DefaultTimeout
	// DefaultTxTimeout is the timeout for transactions.
	DefaultTxTimeout = 30 * time.Second
)

// FilterEncoding supplies database or collection overrides when preparing a
// read filter. GetEntity and GetEntities also use it to decode results when
// the codec settings are known. Nil fields inherit the Atlas client's settings
// or use the Atlas registry and default BSON options on an external client.
// With both fields nil, borrowed handles use the driver's decoder and query
// independently.
type FilterEncoding struct {
	Registry    *bson.Registry
	BSONOptions *options.BSONOptions

	// FreshRead bypasses deduplication for GetEntity and GetEntities.
	FreshRead bool
}

func prepareFilter(ctx context.Context, coll *mongo.Collection, filter any, settings ...FilterEncoding) (bson.Raw, error) {
	var encoding FilterEncoding
	if len(settings) > 0 {
		encoding = settings[len(settings)-1]
	}
	reg, opts := query.FilterEncoding(coll, encoding.Registry, encoding.BSONOptions)
	return bsonfilter.Prepare(ctx, filter, reg, opts)
}

func generateDeduplicationKey(prefix string, filter bson.Raw) string {
	return corehash.SHA256HexString(prefix + ":" + string(filter))
}

// capListOffset enforces the [MaxListOffset] cap on offset-based pagination.
// `$skip` cost is linear in offset because MongoDB has to read and discard
// every skipped document, so accepting an arbitrary value is a cheap way
// for a hostile client to overload the server. Past the cap the value is
// clamped and a warning is logged so the operator can switch to ListCursor
// (cursor pagination is O(1) per page).
//
// Parameters:
//   - offset: The requested $skip value
//   - logger: Logger for warning messages (can be DiscardLogger to suppress warnings)
//
// Returns:
//   - int64: The original offset if <= [MaxListOffset], otherwise [MaxListOffset]
func capListOffset(offset int64, logger *slog.Logger) int64 {
	if offset > MaxListOffset {
		logger.Warn("large list offset request capped at maximum",
			"requested_offset", offset,
			slog.Int("enforced_offset", MaxListOffset),
			slog.Int("max_allowed", MaxListOffset),
			slog.String("recommendation", "use ListCursor for deep pagination"),
		)
		return MaxListOffset
	}
	return offset
}

// capListLimit enforces the MaxListLimit constraint to prevent excessive memory usage.
// Large limit values can cause memory pressure, slow response times, and potential
// out-of-memory errors. This function caps the limit and logs a warning when enforcement occurs.
//
// Parameters:
//   - limit: The requested page size limit
//   - logger: Logger for warning messages (can be DiscardLogger to suppress warnings)
//
// Returns:
//   - int64: The original limit if <= MaxListLimit, otherwise MaxListLimit (1000)
//
// When capping occurs, a warning is logged with:
//   - requested_limit: The value that was requested
//   - enforced_limit: The actual limit being applied (MaxListLimit)
//   - max_allowed: The maximum allowed value for reference
//
// Use case: Protecting the system from excessive resource consumption while maintaining
// transparency through logging. Callers requesting very large pages are notified of the cap.
func capListLimit(limit int64, logger *slog.Logger) int64 {
	if limit > MaxListLimit {
		logger.Warn("large limit request capped at maximum",
			"requested_limit", limit,
			slog.Int("enforced_limit", MaxListLimit),
			slog.Int("max_allowed", MaxListLimit),
		)
		return MaxListLimit
	}
	return limit
}

// logHintUsage logs index hint usage for query performance monitoring and debugging.
// This function provides visibility into index hint usage patterns, which is critical
// for identifying when manual query optimization is being applied.
//
// Index hints override MongoDB's query planner, so tracking their usage helps:
//   - Monitor which queries require manual optimization
//   - Identify hint patterns during performance troubleshooting
//   - Audit forced index usage across the application
//
// Parameters:
//   - logger: Logger instance for outputting hint usage information
//   - hint: Index hint (can be string name, bson.D, or bson.M), no-op if nil
//   - collectionName: The MongoDB collection being queried
//   - filter: The query filter to show which fields are being filtered
//   - extraFields: Optional additional fields to include in the log (key-value pairs)
//
// Logged fields:
//   - hint: The index hint being used
//   - collection: Target collection name
//   - filter_fields: Extracted field names from the filter (excluding $ operators)
//   - Any extra fields provided by the caller
//
// Example log output:
//
//	INFO using index hint for query hint=user_active_1_created_at_-1
//	  collection=users filter_fields=[active,department] cursor_provided=true
//
// Use case: Debugging slow queries, performance regression analysis, and ensuring
// index hints are being applied correctly in production.
func logHintUsage(logger *slog.Logger, hint any, collectionName string, filter bson.Raw, extraFields ...any) {
	if hint == nil {
		return
	}

	baseFields := make([]any, 0, 6+len(extraFields))
	baseFields = append(baseFields,
		"hint", hint,
		"collection", collectionName,
		"filter_fields", getFilterFields(filter),
	)

	logger.Info("using index hint for query",
		append(baseFields, extraFields...)...,
	)
}

// nextPowerOfTwo returns the next power of two greater than or equal to n.
// Powers of two are optimal capacities for hash tables (like maps) because they
// minimize collisions and enable efficient modulo operations using bitwise AND.
//
// This function uses bit manipulation for O(1) performance:
//  1. For n <= 1, returns 1 (smallest power of two)
//  2. Otherwise, uses bits.Len to find the position of the highest set bit in (n-1)
//  3. Returns 1 << position (equivalent to 2^position)
//
// Parameters:
//   - n: Input value (can be negative, treated as 1)
//
// Returns:
//   - int: Smallest power of two >= n
//
// Examples:
//   - nextPowerOfTwo(0) → 1
//   - nextPowerOfTwo(1) → 1
//   - nextPowerOfTwo(5) → 8
//   - nextPowerOfTwo(8) → 8
//   - nextPowerOfTwo(100) → 128
//
// Use case: Pre-allocating map capacity to the next power of two reduces resizing
// operations and improves performance for growing collections. This is used in
// BuildFilter to optimize bson.M allocation.
func nextPowerOfTwo(n int) int {
	if n <= 1 {
		return 1
	}
	return 1 << bits.Len(uint(n-1)) // #nosec G115 -- n is validated > 1
}

// BsonLookup retrieves a value from a BSON document using dot notation for nested paths.
// Supports both bson.M and bson.D document types at any nesting level.
// Returns nil if the key is not found or any intermediate path segment is missing.
//
// Example:
//
//	doc := bson.M{"sort_fields": bson.D{{Key: "value", Value: "Agent"}}}
//	BsonLookup(doc, "sort_fields.value") // returns "Agent"
//	BsonLookup(doc, "sort_fields")       // returns bson.D{{Key: "value", Value: "Agent"}}
//	BsonLookup(doc, "missing.key")       // returns nil
func BsonLookup(doc any, key string) any {
	current := doc
	for part := range strings.SplitSeq(key, ".") {
		current = bsonFieldValue(current, part)
		if current == nil {
			return nil
		}
	}
	return current
}

// bsonFieldValue retrieves a single field value from a BSON document by field name.
// Handles bson.M (map lookup) and bson.D (ordered element scan).
func bsonFieldValue(doc any, field string) any {
	switch d := doc.(type) {
	case bson.M:
		return d[field]
	case bson.D:
		for _, elem := range d {
			if elem.Key == field {
				return elem.Value
			}
		}
	}
	return nil
}
