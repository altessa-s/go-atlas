// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package parser

import coremaps "github.com/altessa-s/go-atlas/core/collections/maps"

type parsedFieldTags struct {
	Default   string
	Metadata  map[string]string
	Modifiers []string
	Checks    map[string]string
}

func parseFieldTags(tagStr string) (parsedFieldTags, error) {
	var out parsedFieldTags

	if genTag := ParseTag(tagStr, "optgen"); genTag != "" {
		genDefault, genMeta := ParseOptGen(genTag)
		out.Default = genDefault
		out.Metadata = coremaps.Merge(genMeta, map[string]string(nil))
	}

	if valTag := ParseTag(tagStr, "optval"); valTag != "" {
		out.Modifiers = ParseOptModifiers(valTag)
	}

	checks, err := parseOptChecks(ParseTag(tagStr, "optcheck"))
	if err != nil {
		return parsedFieldTags{}, err
	}
	out.Checks = checks

	return out, nil
}
