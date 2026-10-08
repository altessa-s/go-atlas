// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/domain/behavior"
)

// projectionTranslator folds a resolved behavior Object into an exclusion
// projection: a bson.M mapping the dot-notation path of every stripped field to
// 0. It implements [behavior.Translator].
//
// A projectionTranslator holds no per-call state and is safe for concurrent use.
type projectionTranslator struct {
	o *options
}

// NewProjectionTranslator returns a [behavior.Translator] that builds an
// exclusion projection ({path: 0}) for every field the engine marked Strip,
// using dot-notation paths for nested fields. Drive it through
// [github.com/altessa-s/go-atlas/domain/behavior.New] with
// behavior.WithKinds(behavior.DefaultResponseKinds...) to exclude InputOnly
// fields from query results.
//
// Projection is type-driven: nested paths must appear even when a pointer is nil
// or a collection is empty at run time. The engine MUST therefore be built with
// [github.com/altessa-s/go-atlas/domain/behavior.WithSchemaWalk] so absent nested
// struct types are still resolved; pass a zero value as the type carrier:
//
//	proj := behavior.New[bson.M](mongo.NewProjectionTranslator(),
//	    behavior.WithKinds(behavior.DefaultResponseKinds...), behavior.WithSchemaWalk())
//	p, err := proj.Translate(ctx, Entity{})
//
// For slices of structs the field path is emitted without an index
// ("items.secret"): MongoDB applies the exclusion across every element. Map
// values sit under dynamic keys ("labels.<key>.secret") no path can address,
// so a map whose values carry a stripped field is excluded whole ("labels").
func NewProjectionTranslator(opts ...Option) behavior.Translator[bson.M] {
	return &projectionTranslator{o: newOptions(opts...)}
}

// Translate folds root into an exclusion projection. The returned map is empty
// (non-nil) when no field matches. A map whose values carry a stripped field
// is excluded whole, since its keys are dynamic. It fails with
// [ErrStrippedInline] for stripped data in a `bson:",inline"` field.
func (t *projectionTranslator) Translate(_ context.Context, root behavior.Object) (bson.M, error) {
	var ps pathSet
	if err := t.o.collectPaths(root, "", &ps); err != nil {
		return nil, err
	}
	out := make(bson.M, len(ps.denied))
	for _, p := range ps.denied {
		out[p] = 0
	}
	return out, nil
}

// joinPath joins prefix and name with a dot; an empty prefix yields name.
func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}
