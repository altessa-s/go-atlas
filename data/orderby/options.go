// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package orderby

import (
	"cmp"
	"slices"
	"strings"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=translatorOptions --output=translator_options_gen.go --option-type=TranslatorOption

// translatorOptions is the optgen target — a private struct that holds
// the configuration shared by every database translator in this package.
// The struct is unexported on purpose so callers cannot bypass the
// constructor; cross-package access happens through the public
// [TranslatorContext] type alias and the read methods defined on
// *translatorOptions below.
//
// allowedFields and fieldMapping are stored as [coremaps.ImmutableMap]
// — they are built once by [WithAllowedFields] / [WithFieldMapping] and
// only read afterwards, which is precisely the build-once-read-many
// shape AGENTS.md mandates ImmutableMap for. The prefix collections
// (allowedFieldPrefixes, fieldPrefixMapping) stay as plain sorted slices
// because they need longest/linear scan rather than hash lookup. All
// option setters except [WithUntrustedInput] are hand-written because
// optgen cannot express the variadic / map-to-ImmutableMap / slice-of-
// struct conversions.
type translatorOptions struct {
	allowedFields        *coremaps.ImmutableMap[string, struct{}] `opt:"-"`
	allowedFieldPrefixes []string                                 `opt:"-"`
	fieldMapping         *coremaps.ImmutableMap[string, string]   `opt:"-"`
	fieldPrefixMapping   []prefixMapping                          `opt:"-"`
	untrustedInput       bool                                     `opt:"UntrustedInput"`
}

// prefixMapping describes a single (From → To) prefix rewrite produced
// by [WithFieldPrefixMapping]. Both ends always include the trailing dot
// so applying the rewrite cannot accidentally cross a segment boundary.
type prefixMapping struct {
	From string
	To   string
}

// TranslatorContext is the public read-side handle that database
// translator subpackages thread through their Translate calls. It is a
// type alias for the package-private [translatorOptions] struct, so the
// read methods ([TranslatorContext.ApplyFieldMapping],
// [TranslatorContext.IsFieldAllowed]) are accessible cross-package while
// the raw fields stay encapsulated. A *TranslatorContext is always the
// product of a successful [NewTranslatorContext] call — there is no
// allow-list misconfiguration left to surface at translation time.
type TranslatorContext = translatorOptions

// NewTranslatorContext builds a [TranslatorContext] with the given
// options applied and validates the result. It fails with
// [ErrAllowlistRequired] when [WithUntrustedInput] was set but no
// non-empty [WithAllowedFields] allow-list was provided — that
// combination would otherwise silently produce a deny-all translator,
// which is almost always a configuration bug.
//
// Translator constructors call this from their NewTranslator
// implementations and propagate the error. Misconfiguration therefore
// surfaces at process start rather than on the first request.
func NewTranslatorContext(opts ...TranslatorOption) (*TranslatorContext, error) {
	ctx := newTranslatorOptions(opts...)
	if err := ctx.requireAllowlist(); err != nil {
		return nil, err
	}
	return ctx, nil
}

// WithAllowedFields sets a whitelist of allowed field names. Keys are
// matched against [Key.Name] — the raw dotted form — so callers configure
// nested fields exactly as they appear in the DSL ("address.city"). The
// list is frozen into an [coremaps.ImmutableMap] so lookups are
// allocation-free and the policy cannot drift after the translator is
// constructed.
//
// Entries that end in ".*" are treated as wildcard prefixes: every key
// whose dotted path starts with the matching segment is allowed. A bare
// "*" matches anything (equivalent to omitting the allow-list).
// Embedded asterisks (e.g. "foo.*.bar") have no special meaning and are
// stored as exact matches — they will never match a real key. The
// trailing ".*" entry does **not** include the bare prefix itself: to
// allow "address" in addition to "address.X", list both.
//
// Example:
//
//	trans, err := mongo.NewTranslator(orderby.WithAllowedFields(
//	    "createdAt",     // exact
//	    "slug",          // exact
//	    "address.*",     // any subkey of address
//	))
func WithAllowedFields(fields ...string) TranslatorOption {
	return func(o *translatorOptions) {
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
		// Sort prefixes by length descending so overlapping entries
		// ("address.", "address.deep.") have deterministic match order
		// and the longest match wins on the first hit during the
		// IsFieldAllowed linear scan.
		slices.SortFunc(prefixes, func(a, b string) int {
			return cmp.Compare(len(b), len(a))
		})
		o.allowedFields = coremaps.NewImmutableMap(exact)
		o.allowedFieldPrefixes = prefixes
	}
}

// WithFieldMapping sets an exact mapping from DSL field names to database
// column names. This lets callers expose user-friendly names in the API
// surface while sorting on the underlying storage representation. The
// mapping is copied into an [coremaps.ImmutableMap] so subsequent
// mutations of the caller's map cannot affect a constructed translator.
//
// Example:
//
//	trans, err := mongo.NewTranslator(orderby.WithFieldMapping(map[string]string{
//	    "createdAt": "created_at",
//	}))
func WithFieldMapping(mapping map[string]string) TranslatorOption {
	return func(o *translatorOptions) {
		o.fieldMapping = coremaps.NewImmutableMap(mapping)
	}
}

// WithFieldPrefixMapping sets a mapping that rewrites the leading
// segments of a dotted field path. Each key and value must end in "."
// so the rewrite happens on a segment boundary; entries that do not are
// silently ignored — without the trailing dot a key like "addr" would
// match every identifier starting with those letters and corrupt
// unrelated fields.
//
// When multiple prefixes match, the longest one wins. Exact
// [WithFieldMapping] entries always win over any prefix rewrite.
//
// Example:
//
//	trans, err := mongo.NewTranslator(orderby.WithFieldPrefixMapping(map[string]string{
//	    "address.":      "addr.",        // address.city.zip → addr.city.zip
//	    "user.profile.": "users.prof.",  // longest-match wins over "user."
//	}))
func WithFieldPrefixMapping(mapping map[string]string) TranslatorOption {
	return func(o *translatorOptions) {
		pm := make([]prefixMapping, 0, len(mapping))
		for from, to := range mapping {
			pm = coreslices.AppendIf(pm,
				strings.HasSuffix(from, ".") && strings.HasSuffix(to, "."),
				prefixMapping{From: from, To: to},
			)
		}
		slices.SortFunc(pm, func(a, b prefixMapping) int {
			return cmp.Compare(len(b.From), len(a.From))
		})
		o.fieldPrefixMapping = pm
	}
}

// ApplyFieldMapping returns the DB column name for a DSL field. The
// lookup order is: exact mapping (constant-time), then prefix mappings
// (longest first), then the input field unchanged when nothing matches.
func (o *translatorOptions) ApplyFieldMapping(field string) string {
	if o.fieldMapping != nil {
		if mapped, ok := o.fieldMapping.Get(field); ok {
			return mapped
		}
	}
	for _, pm := range o.fieldPrefixMapping {
		if strings.HasPrefix(field, pm.From) {
			return pm.To + field[len(pm.From):]
		}
	}
	return field
}

// IsFieldAllowed reports whether a field is in the allow-list. Returns
// true when no allow-list is configured at all. Checks the exact set
// first, then linear-scans the wildcard prefixes (sorted longest-first
// by [WithAllowedFields], so the first match is the most specific).
//
// The fast path is inlined here rather than delegating to a helper so
// the common "no allow-list" case stays a single nil/len check on the
// translation hot path.
func (o *translatorOptions) IsFieldAllowed(field string) bool {
	if o.allowedFields == nil && len(o.allowedFieldPrefixes) == 0 {
		return true
	}
	if o.allowedFields != nil && o.allowedFields.Contains(field) {
		return true
	}
	for _, p := range o.allowedFieldPrefixes {
		if strings.HasPrefix(field, p) {
			return true
		}
	}
	return false
}

// requireAllowlist returns [ErrAllowlistRequired] when the translator was
// marked as receiving untrusted input ([WithUntrustedInput]) but no
// usable allow-list was configured ([WithAllowedFields]).
// [NewTranslatorContext] runs this once at construction so the check
// disappears from the translation hot path — callers never see a
// misconfigured *TranslatorContext.
//
// An empty allow-list (e.g. `WithAllowedFields()` or `WithAllowedFields(slice...)`
// where the slice happened to be empty) is treated the same as no
// allow-list at all: the deny-all behavior it would otherwise produce
// is almost always a configuration bug — a forgotten append, an empty
// slice from a config loader, a misspelled struct field — and it is
// more useful to fail loudly than to silently reject every sort. A
// wildcard entry counts as a non-empty allow-list.
func (o *translatorOptions) requireAllowlist() error {
	if !o.untrustedInput {
		return nil
	}
	hasExact := o.allowedFields != nil && o.allowedFields.Len() > 0
	hasPrefix := len(o.allowedFieldPrefixes) > 0
	if !hasExact && !hasPrefix {
		return ErrAllowlistRequired
	}
	return nil
}
