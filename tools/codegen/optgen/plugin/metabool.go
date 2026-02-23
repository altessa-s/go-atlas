// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// This file provides utility functions for parsing boolean and modifier values
// from optgen tag strings. Supports common truthy representations (1, true, yes, y, on).

package plugin

import "strings"

// IsTruthyString reports whether s represents a boolean-true value after
// trimming whitespace and lowercasing. Recognized truthy representations:
// "1", "true", "yes", "y", "on". Everything else (including the empty
// string) is considered false.
func IsTruthyString(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

// IsTruthyMetadata reports whether the metadata map md contains key mapped to
// a truthy value (see [IsTruthyString]). Returns false when md is nil or key
// is absent.
func IsTruthyMetadata(md map[string]string, key string) bool {
	if md == nil {
		return false
	}
	v, ok := md[key]
	if !ok {
		return false
	}
	return IsTruthyString(v)
}

// HasModifier reports whether the modifiers slice contains an exact match for modifier.
func HasModifier(modifiers []string, modifier string) bool {
	for _, mod := range modifiers {
		if mod == modifier {
			return true
		}
	}
	return false
}
