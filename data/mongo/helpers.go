// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"fmt"
	"log/slog"
	"math/bits"
	"reflect"
	"slices"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	corehash "github.com/altessa-s/go-atlas/core/encoding/hash"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// discardLogger is a singleton logger that discards all output.
// Reused across defaultListOptions/defaultListCursorOptions to avoid
// allocating a new *slog.Logger per List/ListCursor call.
var discardLogger = slog.New(slog.DiscardHandler)

// Sort order constants for MongoDB sorting
const (
	// SortAscending represents ascending sort order (1)
	SortAscending = 1
	// SortDescending represents descending sort order (-1)
	SortDescending = -1
	// SortPrefixChar is the character used to indicate descending sort
	SortPrefixChar = '-'
)

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

// Deduplication key suffixes for different query types (internal implementation details)
const (
	deduplicationKeySuffixList = "_list"
)

// ParseSortString parses a sort string into a bson.D.
// The sort string is a comma-separated list of field names, with an optional "-" prefix to indicate descending order.
// Example: "name,-age" will be parsed into bson.D{{"name", 1}, {"age", -1}}.
//
// Tokens that resolve to a `$`-prefixed key (e.g. `$natural`) are dropped
// silently: they always force a full collection scan and are never a
// legitimate target for caller-supplied sort strings. Privileged callers
// that really need that pass a literal `bson.D{{Key: "$natural", Value: 1}}`
// directly. For untrusted input prefer [ParseSortStringStrict], which
// also enforces a field allowlist.
func ParseSortString(sortString string) bson.D {
	tokens, ok := splitSortTokens(sortString)
	if !ok {
		return nil
	}

	result := make(bson.D, 0, len(tokens))
	for _, item := range tokens {
		field, order := splitSortToken(item)
		if field == "" || isReservedSortKey(field) {
			continue
		}
		result = append(result, bson.E{Key: field, Value: order})
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// ParseSortStringStrict is the same as [ParseSortString] but additionally
// enforces an allowlist of permitted field names and surfaces a non-nil
// error when the input references anything outside it (including any
// `$`-prefixed operator key).
//
// Use this whenever the sort string originates from external input —
// REST query params, gRPC request fields, anything a hostile client
// controls. Without an allowlist a caller can ask MongoDB to sort on
// arbitrary non-indexed fields, which forces an in-memory sort that
// aborts past 32 MB by default and otherwise spills to disk; both are
// easy DoS vectors.
//
// allowed is treated as a set: order does not matter and duplicates are
// ignored. Passing an empty slice rejects every field.
func ParseSortStringStrict(sortString string, allowed []string) (bson.D, error) {
	tokens, ok := splitSortTokens(sortString)
	if !ok {
		return nil, nil
	}

	allowSet := make(map[string]struct{}, len(allowed))
	for _, f := range allowed {
		allowSet[f] = struct{}{}
	}

	result := make(bson.D, 0, len(tokens))
	for _, item := range tokens {
		field, order := splitSortToken(item)
		if field == "" {
			continue
		}
		if isReservedSortKey(field) {
			return nil, fmt.Errorf("%w: operator key %q", ErrSortFieldNotAllowed, field)
		}
		if _, ok := allowSet[field]; !ok {
			return nil, fmt.Errorf("%w: %q", ErrSortFieldNotAllowed, field)
		}
		result = append(result, bson.E{Key: field, Value: order})
	}

	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

// splitSortTokens normalizes the comma-separated input into the trimmed,
// deduplicated token slice both parsers iterate over.
func splitSortTokens(sortString string) ([]string, bool) {
	parts := corestrings.Split(sortString, corestrings.SplitOptions{
		Separator: ",",
		TrimSpace: true,
		SkipEmpty: true,
	})
	if len(parts) == 0 {
		return nil, false
	}

	out := make([]string, 0, len(parts))
	for _, item := range parts {
		if item == string(SortPrefixChar) {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil, false
	}
	return coreslices.Deduplicate(out), true
}

// splitSortToken returns the field name and the encoded sort direction.
// A leading `-` produces [SortDescending]; otherwise [SortAscending].
func splitSortToken(item string) (string, int) {
	if len(item) > firstElementIndex && item[firstElementIndex] == SortPrefixChar {
		return strings.TrimSpace(item[secondElementIndex:]), SortDescending
	}
	return item, SortAscending
}

// isReservedSortKey reports whether field is a MongoDB operator key
// (starts with `$`). Such keys must never come from untrusted input.
func isReservedSortKey(field string) bool {
	return strings.HasPrefix(field, "$")
}

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
	// DefaultQueryTimeout is the timeout for read operations (find, findOne).
	DefaultQueryTimeout = 5 * time.Second
	// DefaultTxTimeout is the timeout for transactions.
	DefaultTxTimeout = 30 * time.Second
)

// indirectType returns the underlying type by recursively dereferencing pointers.
// This function is useful when working with reflection to get the actual type
// regardless of pointer indirection levels.
//
// Parameters:
//   - t: A reflect.Type that may be a pointer or value type
//
// Returns:
//   - reflect.Type: The underlying non-pointer type
//
// Examples:
//   - *User → User
//   - **User → User
//   - User → User
//
// Use case: Type introspection when you need the base type for comparison or
// instantiation, regardless of how many levels of pointer indirection exist.
func indirectType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// generateDeduplicationKey creates a deterministic SHA-256 hash key from filter parameters.
// This function is used for query deduplication in singleflight patterns to ensure that
// identical queries (same prefix and filter) map to the same deduplication key.
//
// The function ensures deterministic key generation by:
//  1. Sorting filter keys alphabetically (maps have random iteration order in Go)
//  2. Creating a consistent string representation of key-value pairs
//  3. Hashing with SHA-256 to produce collision-resistant keys
//
// Parameters:
//   - prefix: A string prefix to namespace different query types (e.g., "list", "count")
//   - filter: MongoDB filter using bson.M syntax
//
// Returns:
//   - string: SHA-256 hex-encoded hash of "prefix:key1=val1;key2=val2;..."
//   - If filter is empty, returns prefix unchanged
//
// Example:
//   - generateDeduplicationKey("list", bson.M{"age": 25, "name": "John"})
//     → "a1b2c3..." (SHA-256 hash of "list:age=25;name=John")
//
// Use case: In distributed systems with concurrent identical queries, this key enables
// deduplication to ensure only one query executes while others wait for the result.
func generateDeduplicationKey(prefix string, filter bson.M) string {
	if len(filter) == 0 {
		return prefix
	}

	// Sort keys first, then write key=value pairs directly to a builder
	// to avoid intermediate []string allocation and per-key fmt.Sprintf.
	keys := make([]string, 0, len(filter))
	for k := range filter {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	var sb strings.Builder
	sb.WriteString(prefix)
	sb.WriteByte(':')
	for i, key := range keys {
		if i > 0 {
			sb.WriteByte(';')
		}
		sb.WriteString(key)
		sb.WriteByte('=')
		fmt.Fprint(&sb, filter[key])
	}

	// Use SHA-256 for collision-resistant hashing (essential for correct singleflight deduplication)
	return corehash.SHA256HexString(sb.String())
}

// parseSortOption converts various sort input types to MongoDB's bson.D sort specification.
// This generic function provides flexibility by accepting string, *string, or bson.D inputs,
// allowing callers to use whichever format is most convenient.
//
// Supported input types:
//   - string: Comma-separated field names with "-" prefix for descending (e.g., "name,-age")
//   - *string: Pointer to string (useful for optional parameters), dereferenced before parsing
//   - bson.D: Direct MongoDB sort specification, returned as-is if non-empty
//
// Parameters:
//   - sort: Sort specification in any supported format
//
// Returns:
//   - bson.D: Parsed MongoDB sort specification
//   - bool: true if parsing succeeded and result is non-empty, false otherwise
//
// Parsing behavior:
//   - Empty inputs (empty string, nil pointer, empty bson.D) return (nil, false)
//   - String parsing delegates to ParseSortString for field extraction
//   - Returns false for invalid or empty results
//
// Examples:
//   - parseSortOption("name,-age") → bson.D{{"name", 1}, {"age", -1}}, true
//   - parseSortOption(bson.D{{"created_at", -1}}) → bson.D{{"created_at", -1}}, true
//   - parseSortOption("") → nil, false
func parseSortOption[T ~string | ~*string | bson.D](sort T) (bson.D, bool) {
	switch t := any(sort).(type) {
	case *string:
		if t != nil {
			if parsed := ParseSortString(*t); len(parsed) > 0 {
				return parsed, true
			}
		}
	case string:
		if parsed := ParseSortString(t); len(parsed) > 0 {
			return parsed, true
		}
	case bson.D:
		if len(t) > 0 {
			return t, true
		}
	}
	return nil, false
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
func logHintUsage(logger *slog.Logger, hint any, collectionName string, filter bson.M, extraFields ...any) {
	if hint == nil {
		return
	}

	baseFields := []any{
		"hint", hint,
		"collection", collectionName,
		"filter_fields", getFilterFields(filter),
	}

	logger.Info("using index hint for query",
		append(baseFields, extraFields...)...,
	)
}

// appendFacetResultTransform normalizes facet aggregation results into a consistent structure.
// MongoDB's $facet stage returns results in a nested structure like:
//
//	{items: [{...}, {...}], count: [{total: 100}]}
//
// This function appends pipeline stages that transform it into a flat, predictable format:
//
//	{items: [{...}, {...}], total: 100}
//
// Parameters:
//   - pipeline: The existing aggregation pipeline (must end with a $facet stage)
//   - includeTotal: Whether the facet includes a count branch
//
// Returns:
//   - bson.A: Pipeline with additional $unwind and $project stages appended
//
// Transformation logic:
//   - If includeTotal is true:
//     1. $unwind the count array (preserving null/empty arrays)
//     2. $project to extract count.total with $ifNull default to 0
//   - If includeTotal is false:
//     1. $project to set total to -1 (indicates count was not computed)
//
// The resulting structure is:
//   - items: Array of documents (always present, may be empty)
//   - total: Integer count (>=0 if includeTotal true, -1 if false)
//
// Use case: Standardizing pagination results across list operations for consistent
// API responses and easier client-side handling.
func appendFacetResultTransform(pipeline bson.A, includeTotal bool) bson.A {
	if includeTotal {
		return append(pipeline,
			bson.M{"$unwind": bson.M{
				"path":                       "$count",
				"preserveNullAndEmptyArrays": true,
			}},
			bson.M{"$project": bson.M{
				"total": bson.M{"$ifNull": bson.A{"$count.total", 0}},
				"items": "$items",
			}},
		)
	}

	return append(pipeline,
		bson.M{"$project": bson.M{
			"total": -1,
			"items": "$items",
		}},
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
//	doc := bson.M{"sort_fields": bson.D{{Key: "value", Value: "Агент"}}}
//	BsonLookup(doc, "sort_fields.value") // returns "Агент"
//	BsonLookup(doc, "sort_fields")       // returns bson.D{{Key: "value", Value: "Агент"}}
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
