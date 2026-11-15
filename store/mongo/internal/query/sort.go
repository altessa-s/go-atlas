// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package query

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	corestrings "github.com/altessa-s/go-atlas-core/text/strings"
)

// Sort order constants for MongoDB sorting
const (
	// SortAscending represents ascending sort order (1)
	SortAscending = 1
	// SortDescending represents descending sort order (-1)
	SortDescending = -1
	// SortPrefixChar is the character used to indicate descending sort
	SortPrefixChar = '-'
)

// ErrSortFieldNotAllowed identifies a field outside the requested allowlist.
var ErrSortFieldNotAllowed = errors.New("sort field not allowed")

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
	if len(item) > 0 && item[0] == SortPrefixChar {
		return strings.TrimSpace(item[1:]), SortDescending
	}
	return item, SortAscending
}

// isReservedSortKey reports whether field is a MongoDB operator key
// (starts with `$`). Such keys must never come from untrusted input.
func isReservedSortKey(field string) bool {
	return strings.HasPrefix(field, "$")
}

// ParseSortOption normalizes the sort forms accepted by List and ListCursor.
// Named string and pointer types follow the same rules as their underlying types.
func ParseSortOption[T ~string | ~*string | bson.D](sort T) (bson.D, bool) {
	if doc, ok := any(sort).(bson.D); ok {
		if len(doc) == 0 {
			return nil, false
		}
		return doc, true
	}
	value := reflect.ValueOf(sort)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, false
		}
		value = value.Elem()
	}
	parsed := ParseSortString(value.String())
	return parsed, len(parsed) > 0
}
