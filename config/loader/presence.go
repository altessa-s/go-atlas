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
// the generic decode of the merged file contents (mappings, sequences and
// scalars), so a key that exists in it was written by the operator even when
// its value is false, 0 or "". Default tags are applied only to fields absent
// from it.
type presence struct {
	node any
	// fold matches struct field keys case-insensitively, as the TOML decoder
	// does; the YAML decoder matches exactly.
	fold bool
}

// normalizePresence converts the mapping types produced by the decoders to
// map[string]any once, so lookups never re-normalize.
func normalizePresence(node any) any {
	switch n := node.(type) {
	case map[string]any:
		for k, v := range n {
			n[k] = normalizePresence(v)
		}
		return n
	case map[any]any:
		out := make(map[string]any, len(n))
		for k, v := range n {
			out[fmt.Sprint(k)] = normalizePresence(v)
		}
		return out
	case []any:
		for i, v := range n {
			n[i] = normalizePresence(v)
		}
		return n
	case []map[string]any:
		out := make([]any, len(n))
		for i, v := range n {
			out[i] = normalizePresence(v)
		}
		return out
	default:
		return node
	}
}

// mergePresence overlays src onto dst the way a later file decode overlays the
// configuration of type t: struct fields merge key by key, a map entry or any
// other value written by the later file replaces the earlier one wholesale.
func (cf *Config) mergePresence(dst, src any, t reflect.Type) any {
	dm, dok := dst.(map[string]any)
	sm, sok := src.(map[string]any)
	if !dok || !sok || t == nil {
		return src
	}

	t = indirectType(t)
	out := make(map[string]any, len(dm)+len(sm))
	for k, v := range dm {
		out[k] = v
	}
	for k, v := range sm {
		if t.Kind() != reflect.Struct {
			out[k] = v // map entries are replaced, not merged
			continue
		}
		ft, ok := cf.fieldTypeForKey(t, k)
		if !ok {
			out[k] = v
			continue
		}
		out[k] = cf.mergePresence(out[k], v, ft)
	}
	return out
}

// fieldTypeForKey resolves the type of the struct field a file key binds to,
// descending into inline fields.
func (cf *Config) fieldTypeForKey(t reflect.Type, key string) (reflect.Type, bool) {
	tagName, fold := cf.backend.StructTagName(), cf.foldKeys()
	for i := range t.NumField() {
		sf := t.Field(i)
		if isInline(sf, tagName) {
			if it := indirectType(sf.Type); it.Kind() == reflect.Struct {
				if ft, ok := cf.fieldTypeForKey(it, key); ok {
					return ft, true
				}
			}
			continue
		}
		if name := fileKey(sf, tagName); name != "" && keyMatches(name, key, fold) {
			return sf.Type, true
		}
	}
	return nil, false
}

// foldKeys reports whether the backend binds struct field keys
// case-insensitively.
func (cf *Config) foldKeys() bool {
	return cf.backend.StructTagName() == "toml"
}

func keyMatches(fieldKey, fileKey string, fold bool) bool {
	if fold {
		return strings.EqualFold(fieldKey, fileKey)
	}
	return fieldKey == fileKey
}

// field returns the presence node bound to a struct field key and whether the
// key was set.
func (p presence) field(key string) (presence, bool) {
	m, ok := p.node.(map[string]any)
	if !ok {
		return presence{fold: p.fold}, false
	}
	if v, ok := m[key]; ok {
		return presence{node: v, fold: p.fold}, true
	}
	if p.fold {
		for k, v := range m {
			if strings.EqualFold(k, key) {
				return presence{node: v, fold: p.fold}, true
			}
		}
	}
	return presence{fold: p.fold}, false
}

// entry returns the presence node of a map entry; map keys match exactly.
func (p presence) entry(key string) presence {
	if m, ok := p.node.(map[string]any); ok {
		if v, ok := m[key]; ok {
			return presence{node: v, fold: p.fold}
		}
	}
	return presence{fold: p.fold}
}

// index returns the presence node for element i of a sequence.
func (p presence) index(i int) presence {
	if s, ok := p.node.([]any); ok && i < len(s) {
		return presence{node: s[i], fold: p.fold}
	}
	return presence{fold: p.fold}
}

// fileKey returns the configuration-file key of a struct field for the given
// struct tag, or "" when the field is excluded from files ("-"). An untagged
// field binds to its lowercased name, as the YAML decoder does; the TOML
// decoder matches it case-insensitively anyway.
func fileKey(sf reflect.StructField, tagName string) string {
	name, _, _ := strings.Cut(sf.Tag.Get(tagName), ",")
	switch name {
	case "-":
		return ""
	case "":
		return strings.ToLower(sf.Name)
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
