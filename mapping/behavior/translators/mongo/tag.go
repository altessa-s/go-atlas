// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"reflect"
	"strings"
	"sync"
)

// bsonTagInfo is the parsed form of one bson tag, memoized in bsonTagCache.
type bsonTagInfo struct {
	name         string
	omitEmpty    bool
	omitOnUpdate bool
	minSize      bool
	inline       bool
}

// bsonTagKey identifies a parse: the Go field name participates because it is
// the fallback document name when the tag names no field.
type bsonTagKey struct{ tag, goName string }

// bsonTagCache memoizes parseBSONTag results. Tags and field names come from
// struct definitions, so the key space is finite and an unbounded sync.Map is
// sufficient; the cache removes a per-field strings.Split on every Translate.
var bsonTagCache sync.Map // map[bsonTagKey]bsonTagInfo

// parseBSONTag extracts the document field name and modifiers from a bson tag,
// falling back to the lowercased Go field name when the tag is absent or names no
// field. A "-" or empty tag name yields an empty fieldName (skip).
func parseBSONTag(tag, goName string) (fieldName string, omitEmpty, omitOnUpdate bool) {
	info := bsonTag(tag, goName)
	return info.name, info.omitEmpty, info.omitOnUpdate
}

// fieldTag returns the tag string the codec reads for a field: the value of
// the configured tag key, or — for the default bson key, when it is absent —
// the whole struct tag if it holds no key:value pair (the codec's legacy
// bare-tag form, `Secret string "private"`).
func (o *options) fieldTag(st reflect.StructTag) string {
	tag, ok := st.Lookup(o.bsonTagName)
	if !ok && o.bsonTagName == DefaultBSONTagName && len(st) > 0 && !strings.Contains(string(st), ":") {
		return string(st)
	}
	return tag
}

// bsonTag parses and memoizes a bson tag; see [parseBSONTag].
func bsonTag(tag, goName string) bsonTagInfo {
	key := bsonTagKey{tag: tag, goName: goName}
	if v, ok := bsonTagCache.Load(key); ok {
		info, _ := v.(bsonTagInfo)
		return info
	}

	// As in the codec (struct_tag_parser.go parseTags): only the complete
	// tag "-" skips the field; options are recognized in every token, the
	// first one included, and a non-empty first token names the field.
	var info bsonTagInfo
	if tag != "-" {
		info.name = strings.ToLower(goName)
		for idx, tok := range strings.Split(tag, ",") {
			if idx == 0 && tok != "" {
				info.name = tok
			}
			switch tok {
			case "omitempty":
				info.omitEmpty = true
			case "omitonupdate":
				info.omitOnUpdate = true
			case "minsize":
				info.minSize = true
			case "inline":
				info.inline = true
			}
		}
	}

	bsonTagCache.Store(key, info)
	return info
}
