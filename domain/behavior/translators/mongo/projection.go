// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
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
// For slices and maps of structs the field path is emitted without an index
// ("items.secret", "labels.secret"): MongoDB applies the exclusion across every
// embedded document reachable by that path.
func NewProjectionTranslator(opts ...Option) behavior.Translator[bson.M] {
	return &projectionTranslator{o: newOptions(opts...)}
}

// Translate folds root into an exclusion projection. The returned map is empty
// (non-nil) when no field matches.
func (t *projectionTranslator) Translate(_ context.Context, root behavior.Object) (bson.M, error) {
	out := bson.M{}
	t.project(root, "", out)
	return out, nil
}

// project walks obj, appending an exclusion entry for every stripped field and
// recursing into nested struct/slice/map element schemas. With the engine in
// schema-walk mode, f.Nested and f.Collection are populated from the declared
// type even when the runtime value is absent, so a single representative element
// suffices to enumerate every nested path.
func (t *projectionTranslator) project(obj behavior.Object, prefix string, out bson.M) {
	for i := range obj.Fields {
		f := &obj.Fields[i]

		// A synthetic unnamed field wraps a nested-collection element; it carries
		// no document name of its own, so pass the prefix through unchanged.
		path := prefix
		if f.Name != "" {
			name, _, _ := parseBSONTag(f.Tag.Get(t.o.bsonTagName), f.Name)
			if name == "" {
				continue
			}
			path = joinPath(prefix, name)
		}

		if f.Strip {
			out[path] = 0
			continue
		}

		if f.Nested != nil {
			t.project(*f.Nested, path, out)
			continue
		}
		// One representative element (schema-walk) is enough: the path carries no
		// index, so the same exclusion covers every element of the collection.
		if c := f.Collection; c != nil && len(c.Items) > 0 {
			t.project(c.Items[0], path, out)
		}
	}
}

// joinPath joins prefix and name with a dot; an empty prefix yields name.
func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}
