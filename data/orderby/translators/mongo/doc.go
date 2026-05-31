// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongo translates an [orderby.Spec] into a MongoDB [bson.D]
// sort document.
//
// The result is a [bson.D] (not [bson.M]) because Mongo respects sort key
// precedence only on ordered documents. Ascending keys emit a value of
// int32(1), descending keys emit int32(-1), matching the values the
// MongoDB driver expects in the `sort` option. An empty [orderby.Spec]
// translates to a nil [bson.D] — `SetSort(nil)` is valid and equivalent
// to omitting the sort.
//
// # Basic Usage
//
//	parser, _ := orderby.NewParser()
//	ob, _ := parser.Parse(ctx, "create_time desc, slug")
//
//	trans, err := mongo.NewTranslator()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	sort, _ := trans.Translate(ob)
//	// sort == bson.D{{"create_time", -1}, {"slug", 1}}
//
//	col.Find(ctx, filter, options.Find().SetSort(sort))
//
// # With Options
//
//	trans, err := mongo.NewTranslator(
//	    orderby.WithAllowedFields("createdAt", "slug"),
//	    orderby.WithFieldMapping(map[string]string{
//	        "createdAt": "created_at",
//	    }),
//	    orderby.WithUntrustedInput(),
//	)
//
// # Errors
//
// Returns [orderby.ErrFieldNotAllowed] when an entry references a field
// absent from the configured allow-list, and
// [orderby.ErrAllowlistRequired] when [orderby.WithUntrustedInput] was
// configured without [orderby.WithAllowedFields].
package mongo
