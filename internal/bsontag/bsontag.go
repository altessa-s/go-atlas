// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsontag

import (
	"reflect"
	"strings"
	"sync"
)

// Key is the driver's default struct-tag key, the only one with the bare-tag form.
const Key = "bson"

// Info describes the document key and the encoding and update options used by Atlas.
type Info struct {
	// Name is the document key. As in the driver, a tag such as "-,omitempty" yields the key "-".
	Name string
	// Skip is set only by the complete tag "-"; the other fields are then zero.
	Skip         bool
	OmitEmpty    bool
	OmitOnUpdate bool
	MinSize      bool
	Inline       bool
}

// Lookup reads key from tag. The default [Key] also accepts a bare tag without a colon.
func Lookup(tag reflect.StructTag, key string) string {
	value, ok := tag.Lookup(key)
	if !ok && key == Key && !strings.Contains(string(tag), ":") {
		return string(tag)
	}
	return value
}

type cacheKey struct{ tag, goName string }

// Field declarations repeat across documents. Cache by both inputs because an unnamed tag uses the Go name.
var cache sync.Map // map[cacheKey]Info

// Parse resolves the field name and recognizes options in every token, including the first.
func Parse(tag, goName string) Info {
	key := cacheKey{tag, goName}
	if value, ok := cache.Load(key); ok {
		info, _ := value.(Info)
		return info
	}
	info := parse(tag, goName)
	cache.Store(key, info)
	return info
}

func parse(tag, goName string) Info {
	if tag == "-" {
		return Info{Skip: true}
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		name = strings.ToLower(goName)
	}
	info := Info{Name: name}
	for tok := range strings.SplitSeq(tag, ",") {
		switch tok {
		case "omitempty":
			info.OmitEmpty = true
		case "omitonupdate":
			info.OmitOnUpdate = true
		case "minsize":
			info.MinSize = true
		case "inline":
			info.Inline = true
		}
	}
	return info
}
