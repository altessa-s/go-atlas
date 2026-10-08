// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldpolicy

import (
	"cmp"
	"slices"
	"strings"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

// AllowList is a frozen set of allowed dotted field paths. The zero value
// is "not configured" and allows every field.
//
// Entries ending in ".*" are wildcard prefixes that match every subpath
// but not the bare parent; a lone "*" matches anything. Embedded asterisks
// ("foo.*.bar") have no special meaning and are stored as exact entries
// that never match a real path.
type AllowList struct {
	exact    *coremaps.ImmutableMap[string, struct{}]
	prefixes []string // with the trailing dot; "" for a lone "*"; longest first
}

// NewAllowList freezes fields into an [AllowList]. The result is always
// configured, even for an empty fields list — an empty configured list
// allows nothing.
func NewAllowList(fields ...string) AllowList {
	exact := make(map[string]struct{}, len(fields))
	var prefixes []string
	for _, f := range fields {
		if f == "*" {
			prefixes = append(prefixes, "")
			continue
		}
		if strings.HasSuffix(f, ".*") && !strings.Contains(f[:len(f)-2], "*") {
			prefixes = append(prefixes, f[:len(f)-1]) // keep the dot, drop the star
			continue
		}
		exact[f] = struct{}{}
	}
	// Longest first so overlapping entries ("address.", "address.deep.")
	// have a deterministic match order.
	slices.SortFunc(prefixes, func(a, b string) int {
		return cmp.Compare(len(b), len(a))
	})
	return AllowList{exact: coremaps.NewImmutableMap(exact), prefixes: prefixes}
}

// IsConfigured reports whether the list was built by [NewAllowList].
func (a AllowList) IsConfigured() bool {
	return a.exact != nil || len(a.prefixes) > 0
}

// IsEmpty reports whether the list holds no entry at all, configured or not.
func (a AllowList) IsEmpty() bool {
	return (a.exact == nil || a.exact.Len() == 0) && len(a.prefixes) == 0
}

// MatchesAll reports whether the list allows every path: it is not
// configured, or it contains a lone "*".
func (a AllowList) MatchesAll() bool {
	return !a.IsConfigured() || slices.Contains(a.prefixes, "")
}

// Allows reports whether field is allowed. An unconfigured list allows
// every field.
func (a AllowList) Allows(field string) bool {
	if !a.IsConfigured() {
		return true
	}
	if a.exact != nil && a.exact.Contains(field) {
		return true
	}
	for _, p := range a.prefixes {
		if strings.HasPrefix(field, p) {
			return true
		}
	}
	return false
}

// Roots returns the paths that together cover everything the list allows:
// every exact entry plus the parent of every wildcard prefix ("address.*"
// contributes "address"), sorted and deduplicated. It returns nil when the
// list matches every path, since no finite set of roots covers that.
func (a AllowList) Roots() []string {
	if a.MatchesAll() {
		return nil
	}
	roots := make([]string, 0, a.exact.Len()+len(a.prefixes))
	for k := range a.exact.All() {
		roots = append(roots, k)
	}
	for _, p := range a.prefixes {
		roots = append(roots, strings.TrimSuffix(p, "."))
	}
	slices.Sort(roots)
	return slices.Compact(roots)
}

// prefixRule is a single (From → To) prefix rewrite. Both ends include the
// trailing dot so a rewrite cannot cross a segment boundary.
type prefixRule struct {
	from string
	to   string
}

// Mapping rewrites DSL field names to storage names: exact entries first,
// then the longest matching prefix rule, otherwise the name is returned
// unchanged. The zero value is the identity mapping.
type Mapping struct {
	exact    *coremaps.ImmutableMap[string, string]
	prefixes []prefixRule
	// dotKeys records an exact key ending in ".", which [Mapping.Subtree]
	// treats as renaming the subtree under that key; without one, root skips
	// the "path." lookup and its allocation.
	dotKeys bool
}

// WithExact returns a copy of m whose exact entries are replaced by a
// frozen copy of mapping.
func (m Mapping) WithExact(mapping map[string]string) Mapping {
	m.exact = coremaps.NewImmutableMap(mapping)
	m.dotKeys = false
	for k := range mapping {
		if strings.HasSuffix(k, ".") {
			m.dotKeys = true
			break
		}
	}
	return m
}

// WithPrefixes returns a copy of m whose prefix rules are replaced by
// mapping. Keys and values must both end in "."; entries that do not are
// ignored, because without the trailing dot a key like "addr" would match
// every identifier starting with those letters.
func (m Mapping) WithPrefixes(mapping map[string]string) Mapping {
	rules := make([]prefixRule, 0, len(mapping))
	for from, to := range mapping {
		rules = coreslices.AppendIf(rules,
			strings.HasSuffix(from, ".") && strings.HasSuffix(to, "."),
			prefixRule{from: from, to: to},
		)
	}
	slices.SortFunc(rules, func(a, b prefixRule) int {
		return cmp.Compare(len(b.from), len(a.from))
	})
	m.prefixes = rules
	return m
}

// Apply returns the storage name for field.
func (m Mapping) Apply(field string) string {
	if m.exact != nil {
		if mapped, ok := m.exact.Get(field); ok {
			return mapped
		}
	}
	for _, r := range m.prefixes {
		if rest, ok := strings.CutPrefix(field, r.from); ok {
			return r.to + rest
		}
	}
	return field
}

// Subtree returns the storage paths that together hold path and everything
// under it: the storage name of path itself — where a prefix rule for path's
// own subtree ("address." → "addr.") also renames the parent — plus the
// target of every exact entry and prefix rule strictly under path that the
// mapping moves outside it. Selecting all of them selects the whole subtree;
// selecting only Apply(path) would miss the moved descendants.
func (m Mapping) Subtree(path string) []string {
	return m.AppendSubtree(nil, path)
}

// AppendSubtree appends [Mapping.Subtree] of path to dst and returns the
// extended slice, so a caller resolving many paths reuses one buffer.
func (m Mapping) AppendSubtree(dst []string, path string) []string {
	root := m.root(path)
	start := len(dst)
	dst = append(dst, root)
	add := func(target string) {
		// Overlaps(target, root) with target no shorter than root means
		// target is root or lies under it, so the root already covers it.
		inRoot := len(target) >= len(root) && Overlaps(target, root)
		if !inRoot && !slices.Contains(dst[start:], target) {
			dst = append(dst, target)
		}
	}
	if m.exact != nil {
		for k, v := range m.exact.All() {
			if isUnder(k, path) {
				add(v)
			}
		}
	}
	for _, r := range m.prefixes {
		if isUnder(r.from, path) {
			add(r.to[:len(r.to)-1])
		}
	}
	return dst
}

// root returns the storage name of path as the parent of a subtree: its
// exact entry, else the longest prefix rule matching path itself or one of
// its ancestors, else path. It equals TrimSuffix(Apply(path+"."), ".")
// without building the concatenation.
func (m Mapping) root(path string) string {
	if mapped, ok := m.exactGet(path); ok {
		return mapped
	}
	if m.dotKeys {
		if mapped, ok := m.exactGet(path + "."); ok {
			return strings.TrimSuffix(mapped, ".")
		}
	}
	// Rules are sorted longest first, so the first match of either form is
	// the longest rule Apply(path+".") would pick.
	for _, r := range m.prefixes {
		if len(r.from) == len(path)+1 && strings.HasPrefix(r.from, path) {
			return r.to[:len(r.to)-1]
		}
		if rest, ok := strings.CutPrefix(path, r.from); ok {
			return r.to + rest
		}
	}
	return path
}

// isUnder reports whether k lies under path: path followed by a dot. A prefix
// rule key "path." for path's own subtree counts — its target holds path's
// descendants.
func isUnder(k, path string) bool {
	return len(k) > len(path) && k[len(path)] == '.' && strings.HasPrefix(k, path)
}

// Reach returns every storage path that may hold data of path: its
// [Mapping.Subtree] plus the name path gets through every rule for one of
// its strict ancestors — an exact entry ("credentials" → "c" reaches
// "c.password" for "credentials.password") or a prefix rule — including
// rules that Apply would not pick because an exact entry or a longer prefix
// shadows them. A shadowed rule still says where the ancestor's subtree is
// stored, so it over-approximates on purpose, for deny-lists. The result is
// sorted and deduplicated.
func (m Mapping) Reach(path string) []string {
	out := m.Subtree(path)
	if m.exact != nil {
		for k, v := range m.exact.All() {
			if rest, ok := strings.CutPrefix(path, k+"."); ok {
				out = append(out, v+"."+rest)
			}
		}
	}
	for _, r := range m.prefixes {
		if rest, ok := strings.CutPrefix(path, r.from); ok {
			out = append(out, r.to+rest)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// exactGet looks path up in the exact entries.
func (m Mapping) exactGet(path string) (string, bool) {
	if m.exact == nil {
		return "", false
	}
	return m.exact.Get(path)
}

// Overlaps reports whether the dotted paths a and b are equal or one is an
// ancestor of the other ("a" overlaps "a.b", "a.b" does not overlap "a.bc").
func Overlaps(a, b string) bool {
	switch {
	case len(a) == len(b):
		return a == b
	case len(a) < len(b):
		return b[len(a)] == '.' && strings.HasPrefix(b, a)
	default:
		return a[len(b)] == '.' && strings.HasPrefix(a, b)
	}
}
