// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongo translates a [projection.Spec] into a MongoDB projection
// document, the companion of data/orderby/translators/mongo.
//
// # Basic Usage
//
//	parser, _ := projection.NewParser()
//	spec, _ := parser.Parse(ctx, "name, address.city")
//
//	trans, err := mongo.NewTranslator(
//	    projection.WithUntrustedInput(),
//	    projection.WithAllowedFields("name", "address.*"),
//	    projection.WithRequiredFields("_id"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	proj, _ := trans.Translate(spec)
//	// proj == bson.M{"_id": 1, "address.city": 1, "name": 1}
//
//	res, err := datamongo.ListCursor[Doc](ctx, col, datamongo.WithListCursorProjection(proj))
//
// # Projection semantics
//
// The document is a pure field selection, valid both for an aggregation
// $project stage (used by data/mongo List and ListCursor) and for a find
// projection. No projection operator ($, $slice, $elemMatch) or expression
// is ever emitted.
//
//   - Inclusion {path: 1}: a requested or default path list plus the
//     required fields. MongoDB returns _id unless told otherwise, so the
//     translator adds {_id: 0} when no selected path covers _id.
//   - Exclusion {path: 0}: an empty request under a policy without an
//     allow-list, listing the denied fields. It is never mixed with
//     inclusion.
//   - nil: every field; the data/mongo helpers then add no $project stage.
//
// A path whose ancestor is selected is dropped after the field mapping is
// applied, because MongoDB rejects overlapping paths in one projection
// ("Path collision"). A dotted path into an array of documents
// ("items.name") selects that field in every element; it does not filter
// the elements.
//
// # Pagination
//
// Keyset pagination reads the sort keys back from the last returned row.
// data/mongo ListCursor rejects a projection that drops its cursor or
// primary sort field; list them with [projection.WithRequiredFields].
package mongo
