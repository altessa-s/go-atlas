// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"slices"
	"strings"

	"github.com/altessa-s/go-atlas/domain/behavior"
)

// FieldPaths is the projection policy a behavior-tagged model implies, in
// dot-notation document paths. Both slices are sorted.
type FieldPaths struct {
	// Selectable lists every path a client may request without touching a
	// stripped field: neither the path itself, nor an ancestor, nor a
	// descendant is stripped. A struct that contains a stripped field is
	// therefore absent, while its other fields are listed.
	Selectable []string
	// Denied lists the stripped paths, and every map whose values carry a
	// stripped field — the keys of the exclusion projection
	// [NewProjectionTranslator] builds.
	Denied []string
}

// fieldPathsTranslator folds a resolved behavior Object into [FieldPaths]. It
// implements [behavior.Translator] and holds no per-call state.
type fieldPathsTranslator struct {
	o *options
}

// NewFieldPathsTranslator returns a [behavior.Translator] that enumerates the
// document paths of a model and splits them into selectable and denied ones,
// for configuring data/projection:
//
//	eng := behavior.New[mongo.FieldPaths](mongo.NewFieldPathsTranslator(),
//	    behavior.WithKinds(behavior.DefaultResponseKinds...), behavior.WithSchemaWalk())
//	fp, err := eng.Translate(ctx, User{})
//
//	trans, err := projmongo.NewTranslator(
//	    projection.WithUntrustedInput(),
//	    projection.WithAllowedFields(fp.Selectable...),
//	    projection.WithDeniedStorageFields(fp.Denied...),
//	)
//
// Like [NewProjectionTranslator] it is type-driven, so the engine MUST be
// built with [behavior.WithSchemaWalk]; pass a zero value as the carrier.
// Paths into slices of structs carry no index ("aliases.city"). Map values
// sit under dynamic keys, so no path below a map is selectable, and a map
// whose values carry a stripped field is denied whole. Dynamic element types
// are leaves.
func NewFieldPathsTranslator(opts ...Option) behavior.Translator[FieldPaths] {
	return &fieldPathsTranslator{o: newOptions(opts...)}
}

// Translate folds root into [FieldPaths]. It fails with [ErrStrippedInline]
// for stripped data in a `bson:",inline"` field.
func (t *fieldPathsTranslator) Translate(_ context.Context, root behavior.Object) (FieldPaths, error) {
	var ps pathSet
	if err := t.o.collectPaths(root, "", &ps); err != nil {
		return FieldPaths{}, err
	}
	denied := ps.denied
	selectable := slices.DeleteFunc(ps.all, func(p string) bool {
		return slices.ContainsFunc(denied, func(d string) bool { return overlaps(p, d) })
	})
	slices.Sort(selectable)
	slices.Sort(denied)
	return FieldPaths{Selectable: slices.Compact(selectable), Denied: slices.Compact(denied)}, nil
}

// overlaps reports whether the dotted paths a and b are equal or one is an
// ancestor of the other.
func overlaps(a, b string) bool {
	if len(a) > len(b) {
		a, b = b, a
	}
	return strings.HasPrefix(b, a) && (len(a) == len(b) || b[len(a)] == '.')
}
