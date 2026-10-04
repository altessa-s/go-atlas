// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader

import (
	"fmt"
	"reflect"
	"strings"
)

// presence records which keys the configuration files set explicitly. It is
// the generic decode of the merged file contents (maps, slices and scalars), so
// a key that exists in it was written by the operator even when its value is
// false, 0 or "". Default tags are applied only to fields absent from it.
type presence struct {
	node any
}

// mergePresence deep-merges src into dst the way successive file decodes
// overlay the configuration: mappings merge key by key, anything else from a
// later file replaces the earlier value.
func mergePresence(dst, src any) any {
	dm, dok := asMapping(dst)
	sm, sok := asMapping(src)
	if !dok || !sok {
		return src
	}
	out := make(map[string]any, len(dm)+len(sm))
	for k, v := range dm {
		out[k] = v
	}
	for k, v := range sm {
		out[k] = mergePresence(out[k], v)
	}
	return out
}

// asMapping normalizes the mapping types produced by the YAML and TOML
// decoders to map[string]any.
func asMapping(node any) (map[string]any, bool) {
	switch m := node.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, v := range m {
			out[fmt.Sprint(k)] = v
		}
		return out, true
	default:
		return nil, false
	}
}

// child returns the presence node for key and whether the key was set. Keys
// are matched exactly first and then case-insensitively, mirroring how the
// YAML and TOML decoders bind keys to struct fields.
func (p presence) child(key string) (presence, bool) {
	m, ok := asMapping(p.node)
	if !ok {
		return presence{}, false
	}
	if v, ok := m[key]; ok {
		return presence{node: v}, true
	}
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return presence{node: v}, true
		}
	}
	return presence{}, false
}

// index returns the presence node for element i of a sequence.
func (p presence) index(i int) presence {
	rv := reflect.ValueOf(p.node)
	if rv.Kind() != reflect.Slice || i >= rv.Len() {
		return presence{}
	}
	return presence{node: rv.Index(i).Interface()}
}

// fileKey returns the configuration-file key of a struct field for the given
// struct tag, or "" when the field is excluded from files ("-").
func fileKey(sf reflect.StructField, tagName string) string {
	name, _, _ := strings.Cut(sf.Tag.Get(tagName), ",")
	switch name {
	case "-":
		return ""
	case "":
		return sf.Name
	default:
		return name
	}
}

// isInline reports whether a struct field is flattened into its parent, as the
// loader treats every anonymous struct field.
func isInline(sf reflect.StructField, tagName string) bool {
	if sf.Anonymous {
		return true
	}
	_, opts, _ := strings.Cut(sf.Tag.Get(tagName), ",")
	return strings.Contains(opts, "inline")
}
