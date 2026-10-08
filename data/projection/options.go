// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package projection

import (
	"slices"

	"github.com/altessa-s/go-atlas/data/internal/fieldpolicy"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=translatorOptions --output=translator_options_gen.go --option-type=TranslatorOption

// translatorOptions is the optgen target holding the policy shared by every
// backend translator. Cross-package access goes through the public
// [TranslatorContext] alias and its read methods. All setters except
// [WithUntrustedInput] are hand-written because optgen cannot express the
// variadic / map conversions; the trailing fields are derived once by
// [NewTranslatorContext] and never set by an option.
type translatorOptions struct {
	allowedFields       fieldpolicy.AllowList `opt:"-"`
	fieldMapping        fieldpolicy.Mapping   `opt:"-"`
	deniedFields        []string              `opt:"-"`
	deniedStorageFields []string              `opt:"-"`
	requiredFields      []string              `opt:"-"`
	defaultFields       []string              `opt:"-"`
	untrustedInput      bool                  `opt:"UntrustedInput"`

	// deniedStorage is deniedStorageFields plus every storage path a denied
	// API field reaches through the mapping — its own name, the targets its
	// descendants are moved to, and the names an exact ancestor entry implies
	// — so an allowed alias cannot reach a protected column.
	deniedStorage []string `opt:"-"`
	// defaultSelection is the resolved projection for an empty request.
	defaultSelection Selection `opt:"-"`
}

// TranslatorContext is the read-side handle backend translators thread
// through their Translate calls. A *TranslatorContext is always the product
// of a successful [NewTranslatorContext]: every policy conflict has been
// reported at construction.
type TranslatorContext = translatorOptions

// NewTranslatorContext builds and validates a [TranslatorContext]. It fails
// with:
//
//   - [ErrAllowlistRequired] when [WithUntrustedInput] is set without a
//     non-empty [WithAllowedFields] list;
//   - [ErrInvalidFieldPath] / [ErrUnsupportedPath] for a malformed denied,
//     required or default path;
//   - [ErrConflictingPolicy] when a required field is denied, or a default
//     field is not allowed or is denied;
//   - [ErrDefaultFieldsRequired] when the allow-list is restrictive but its
//     roots cannot serve as the default projection and no
//     [WithDefaultFields] is set.
//
// Misconfiguration therefore surfaces at process start rather than on the
// first request.
func NewTranslatorContext(opts ...TranslatorOption) (*TranslatorContext, error) {
	o := newTranslatorOptions(opts...)
	if o.untrustedInput && o.allowedFields.IsEmpty() {
		return nil, ErrAllowlistRequired
	}
	for _, group := range [][]string{o.deniedFields, o.deniedStorageFields, o.requiredFields, o.defaultFields} {
		for _, p := range group {
			if err := checkPath(p); err != nil {
				return nil, err
			}
		}
	}

	o.deniedStorage = slices.Clone(o.deniedStorageFields)
	for _, d := range o.deniedFields {
		o.deniedStorage = append(o.deniedStorage, o.fieldMapping.Reach(d)...)
	}
	o.deniedStorage = sortedSet(o.deniedStorage)

	for _, r := range o.requiredFields {
		if d, ok := overlapping(r, o.deniedStorage); ok {
			return nil, coreerrs.Wrapf(ErrConflictingPolicy, "required field %q overlaps denied field %q", r, d)
		}
	}

	def, err := o.resolveDefault()
	if err != nil {
		return nil, err
	}
	o.defaultSelection = def
	return o, nil
}

// WithAllowedFields restricts the paths a client may request. Entries are
// matched against the API name; an entry ending in ".*" allows every subpath
// but not the bare parent, and a lone "*" allows anything. A restrictive
// list also defines the default projection — see [WithDefaultFields].
func WithAllowedFields(fields ...string) TranslatorOption {
	return func(o *translatorOptions) {
		o.allowedFields = fieldpolicy.NewAllowList(fields...)
	}
}

// WithFieldMapping sets an exact mapping from API names to storage names.
func WithFieldMapping(mapping map[string]string) TranslatorOption {
	return func(o *translatorOptions) {
		o.fieldMapping = o.fieldMapping.WithExact(mapping)
	}
}

// WithFieldPrefixMapping rewrites the leading segments of dotted API paths
// ("address." → "addr."). Keys and values must end in "."; other entries
// are ignored. The longest prefix wins and exact [WithFieldMapping] entries
// win over any prefix.
func WithFieldPrefixMapping(mapping map[string]string) TranslatorOption {
	return func(o *translatorOptions) {
		o.fieldMapping = o.fieldMapping.WithPrefixes(mapping)
	}
}

// WithDeniedFields lists API paths that are never returned, even when an
// allow-list wildcard covers them. A request for a denied path, one of its
// descendants, or one of its ancestors (which would carry it along) fails
// with [ErrFieldNotAllowed]. Every storage path an entry reaches through the
// mapping — including targets its descendants are mapped to — is denied too.
func WithDeniedFields(paths ...string) TranslatorOption {
	return func(o *translatorOptions) {
		o.deniedFields = slices.Clone(paths)
	}
}

// WithDeniedStorageFields lists storage paths that are never returned,
// whatever API name maps onto them — for example the paths
// domain/behavior/translators/mongo derives from `behavior:"input_only"`
// tags. Matching follows the same ancestor/descendant rule as
// [WithDeniedFields].
func WithDeniedStorageFields(paths ...string) TranslatorOption {
	return func(o *translatorOptions) {
		o.deniedStorageFields = slices.Clone(paths)
	}
}

// WithRequiredFields lists storage paths that every inclusion projection
// carries regardless of the request: the document ID, a tenant key, a
// version used for optimistic locking, or the sort keys a keyset paginator
// reads from the last row. A required path that overlaps a denied one is a
// construction error.
func WithRequiredFields(paths ...string) TranslatorOption {
	return func(o *translatorOptions) {
		o.requiredFields = slices.Clone(paths)
	}
}

// WithDefaultFields sets the API paths returned for an empty request ("" or
// "*"). Each path must pass the allow-list and the deny-list. Without it the
// default is derived: the roots of a restrictive allow-list, or — with no
// allow-list — every field except the denied ones.
func WithDefaultFields(paths ...string) TranslatorOption {
	return func(o *translatorOptions) {
		o.defaultFields = slices.Clone(paths)
	}
}

// DefaultSelection returns the projection used for an empty request. Backends
// that cannot render an exclusion use it to reject the policy at
// construction.
func (o *translatorOptions) DefaultSelection() Selection {
	return o.defaultSelection.clone()
}

// Resolve checks every path of spec against the policy and returns the
// projection in storage names. An empty spec resolves to the default
// selection. Paths are re-validated against the grammar, so a hand-built
// Spec cannot smuggle an operator or an empty segment into the query.
func (o *translatorOptions) Resolve(spec Spec) (Selection, error) {
	if spec.IsEmpty() {
		return o.DefaultSelection(), nil
	}
	include := make([]string, 0, len(spec.Paths)+len(o.requiredFields))
	for _, p := range spec.Paths {
		if err := checkPath(p); err != nil {
			return Selection{}, err
		}
		if !o.allowedFields.Allows(p) {
			return Selection{}, coreerrs.Wrapf(ErrFieldNotAllowed, "%s", p)
		}
		var err error
		if include, err = o.appendStorageNames(include, p); err != nil {
			return Selection{}, err
		}
	}
	include = append(include, o.requiredFields...)
	return Selection{Include: sortedSet(include)}, nil
}

// appendStorageNames applies the deny-lists to the API path p and appends to
// dst the storage paths that hold p's subtree (see
// [fieldpolicy.Mapping.Subtree]).
func (o *translatorOptions) appendStorageNames(dst []string, p string) ([]string, error) {
	if d, ok := overlapping(p, o.deniedFields); ok {
		return nil, coreerrs.Wrapf(ErrFieldNotAllowed, "%s overlaps denied field %s", p, d)
	}
	start := len(dst)
	dst = o.fieldMapping.AppendSubtree(dst, p)
	for _, s := range dst[start:] {
		if d, ok := overlapping(s, o.deniedStorage); ok {
			return nil, coreerrs.Wrapf(ErrFieldNotAllowed, "%s overlaps denied field %s", p, d)
		}
	}
	return dst, nil
}

// resolveDefault derives the selection for an empty request.
func (o *translatorOptions) resolveDefault() (Selection, error) {
	var roots []string
	switch {
	case len(o.defaultFields) > 0:
		for _, p := range o.defaultFields {
			if !o.allowedFields.Allows(p) {
				return Selection{}, coreerrs.Wrapf(ErrConflictingPolicy, "default field %q is not allowed", p)
			}
		}
		roots = o.defaultFields
	case !o.allowedFields.MatchesAll():
		roots = o.allowedFields.Roots()
		if len(roots) == 0 {
			return Selection{}, coreerrs.Wrapf(ErrDefaultFieldsRequired, "the allow-list is empty")
		}
	default:
		// No allow-list: everything except what is denied.
		return Selection{Exclude: slices.Clone(o.deniedStorage)}, nil
	}

	include := make([]string, 0, len(roots)+len(o.requiredFields))
	for _, p := range roots {
		var err error
		if include, err = o.appendStorageNames(include, p); err != nil {
			sentinel := ErrDefaultFieldsRequired
			if len(o.defaultFields) > 0 {
				sentinel = ErrConflictingPolicy
			}
			return Selection{}, coreerrs.Wrapf(sentinel, "default field %q: %v", p, err)
		}
	}
	include = append(include, o.requiredFields...)
	return Selection{Include: sortedSet(include)}, nil
}

// overlapping returns the first entry of set that overlaps path.
func overlapping(path string, set []string) (string, bool) {
	for _, d := range set {
		if fieldpolicy.Overlaps(path, d) {
			return d, true
		}
	}
	return "", false
}

// sortedSet sorts s in place and drops duplicates.
func sortedSet(s []string) []string {
	slices.Sort(s)
	return slices.Compact(s)
}
