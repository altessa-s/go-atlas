// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/data/internal/fieldpolicy"
	"github.com/altessa-s/go-atlas/data/projection"
)

// idField is MongoDB's primary key, which an inclusion projection returns
// unless it is excluded explicitly.
const idField = "_id"

// Projection values for the two modes.
const (
	include int32 = 1
	exclude int32 = 0
)

// Translator converts a [projection.Spec] into a MongoDB projection document
// for an aggregation $project stage or a find projection. It is safe for
// concurrent use after construction.
type Translator struct {
	config *projection.TranslatorContext
}

// NewTranslator creates a MongoDB projection translator. It returns the
// construction errors of [projection.NewTranslatorContext].
func NewTranslator(opts ...projection.TranslatorOption) (*Translator, error) {
	ctx, err := projection.NewTranslatorContext(opts...)
	if err != nil {
		return nil, err
	}
	return &Translator{config: ctx}, nil
}

// Translate resolves spec against the policy and renders it:
//
//   - an inclusion projection {path: 1, …} for a non-empty request or a
//     default derived from an allow-list or WithDefaultFields; _id is
//     suppressed ({_id: 0}) unless a selected path covers it, so list
//     WithRequiredFields("_id") to keep it;
//   - an exclusion projection {path: 0, …} for an empty request under a
//     policy with no allow-list, listing the denied fields;
//   - nil when every field is returned. Callers can pass nil straight to the
//     data/mongo list helpers, which then add no $project stage.
//
// Paths that an ancestor already covers are dropped after the field mapping
// is applied, because MongoDB rejects "a" and "a.b" in one projection as a
// path collision.
func (t *Translator) Translate(spec projection.Spec) (bson.M, error) {
	sel, err := t.config.Resolve(spec)
	if err != nil {
		return nil, err
	}
	switch {
	case len(sel.Include) > 0:
		paths := collapse(sel.Include)
		out := make(bson.M, len(paths)+1)
		for _, p := range paths {
			out[p] = include
		}
		if !slices.ContainsFunc(paths, func(p string) bool { return fieldpolicy.Overlaps(p, idField) }) {
			out[idField] = exclude
		}
		return out, nil
	case len(sel.Exclude) > 0:
		paths := collapse(sel.Exclude)
		out := make(bson.M, len(paths))
		for _, p := range paths {
			out[p] = exclude
		}
		return out, nil
	default:
		return nil, nil //nolint:nilnil // nil is the documented "every field" projection, not a missing value.
	}
}

// collapse drops every path whose ancestor is also present. paths must be
// sorted and deduplicated: an ancestor then precedes its descendants, though
// not necessarily adjacently ("a", "a-b", "a.b"), so a kept path that
// overlaps p is p's ancestor.
func collapse(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if !slices.ContainsFunc(out, func(kept string) bool { return fieldpolicy.Overlaps(p, kept) }) {
			out = append(out, p)
		}
	}
	return out
}
