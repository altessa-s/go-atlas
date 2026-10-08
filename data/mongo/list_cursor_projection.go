// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// idFieldName is MongoDB's primary key, returned by an inclusion projection
// unless excluded explicitly.
const idFieldName = "_id"

// validateCursorProjection rejects a projection that would not return the
// fields [ListCursor] reads back from the last item to build the next
// cursor: cursorIdField and, when the sort starts elsewhere, the primary
// sort field. An empty projection returns every field.
func validateCursorProjection(projection bson.M, cursorIdField string, sort bson.D) error {
	if len(projection) == 0 {
		return nil
	}
	if !projectionKeeps(projection, cursorIdField) {
		return fmt.Errorf("%w: cursor ID field %q", ErrProjectionDropsCursorField, cursorIdField)
	}
	if len(sort) > 0 && sort[0].Key != cursorIdField && !projectionKeeps(projection, sort[0].Key) {
		return fmt.Errorf("%w: sort field %q", ErrProjectionDropsCursorField, sort[0].Key)
	}
	return nil
}

// projectionKeeps reports whether projection returns the stored value of path
// unchanged and in full. The entry for path or its nearest listed ancestor
// decides, and only a plain preservation flag (true or a non-zero number)
// keeps it: an expression — $literal, $$REMOVE, a nested-document
// sub-projection — may replace or drop the value, so it does not count. With
// no such entry an exclusion projection keeps the path and an inclusion
// projection drops it, except _id, which inclusion returns implicitly. A
// listed descendant of path means only part of it survives, which is
// reported as dropped.
func projectionKeeps(projection bson.M, path string) bool {
	var (
		decisive    any
		decisiveLen = -1
	)
	for key, value := range projection {
		switch {
		case key == path || (strings.HasPrefix(path, key) && path[len(key)] == '.'):
			if len(key) > decisiveLen {
				decisive, decisiveLen = value, len(key)
			}
		case strings.HasPrefix(key, path) && key[len(path)] == '.':
			return false
		}
	}
	if decisiveLen >= 0 {
		return isPreservationFlag(decisive)
	}
	if !isInclusionProjection(projection) {
		return true
	}
	return path == idFieldName || strings.HasPrefix(path, idFieldName+".")
}

// isInclusionProjection reports whether MongoDB treats projection as an
// inclusion: some field other than _id is included or computed, or _id is
// the only entry and is included. {_id: 1, password: 0} is an exclusion.
func isInclusionProjection(projection bson.M) bool {
	onlyID := true
	for key, value := range projection {
		if key == idFieldName {
			continue
		}
		onlyID = false
		if !isExclusionValue(value) {
			return true
		}
	}
	id, ok := projection[idFieldName]
	return onlyID && ok && !isExclusionValue(id)
}

// isPreservationFlag reports whether a projection value includes its field
// as stored: true or a non-zero number.
func isPreservationFlag(v any) bool {
	set, isFlag := projectionFlag(v)
	return isFlag && set
}

// isExclusionValue reports whether a projection value excludes its field:
// numeric zero or false. Any other value — 1, true, or an expression —
// returns the field in some form.
func isExclusionValue(v any) bool {
	set, isFlag := projectionFlag(v)
	return isFlag && !set
}

// projectionFlag reads v as a 0/1 projection flag. isFlag is false for an
// expression or any other non-flag value.
func projectionFlag(v any) (set, isFlag bool) {
	switch n := v.(type) {
	case bool:
		return n, true
	case int:
		return n != 0, true
	case int32:
		return n != 0, true
	case int64:
		return n != 0, true
	case float64:
		return n != 0, true
	default:
		return false, false
	}
}
