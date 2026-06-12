// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongo provides [github.com/altessa-s/go-atlas/domain/behavior.Translator]
// implementations that fold behavior-tagged Go structs into MongoDB BSON
// documents from a single behavior reflection walk.
//
// It is a self-contained consumer of the behavior core: it depends only on that
// package and the BSON driver, performs no encryption, and has no dependency on
// data/mongo. The package exposes only translators — drive them through
// [github.com/altessa-s/go-atlas/domain/behavior.New]; configuration that belongs
// to the core (the behavior tag name, traversal depth, and the strip-kind policy)
// is set on the engine, never duplicated here. The translators own only the bson
// tag (see [WithBsonTagName]).
//
// # Translators
//
//   - [NewInsertTranslator] builds an insert document. Each exported field
//     becomes a BSON field named by its bson tag (lowercased Go name as
//     fallback); omitempty drops nil / indirect-zero pointers and empty
//     collections are absent. It applies no behavior-kind filtering, so drive it
//     with no behavior.WithKinds.
//   - [NewUpdateTranslator] builds an update document of the form
//     {"$set": …, "$unset": …}. Nil pointers and empty collections of a
//     non-stripped field go to $unset, nested values are full-replaced in $set,
//     "_id" is removed from $set, and any field the engine marked Strip is
//     excluded from both operators. Drive it with
//     behavior.WithKinds(behavior.DefaultUpdateKinds...).
//   - [NewProjectionTranslator] builds an exclusion projection ({path: 0}) for
//     every stripped field, using dot-notation paths for nested fields. It is
//     type-driven, so the engine MUST be built with behavior.WithSchemaWalk();
//     drive it with behavior.WithKinds(behavior.DefaultResponseKinds...).
//
// # Type handling
//
// bool is written as a BSON bool; []byte / []uint8 as a Binary value; a non-empty
// slice or array of recursable structs as an array of subdocuments; a non-nil
// pointer-to-struct as a nested subdocument; a string-keyed map as a subdocument
// (keys starting with "$" are rejected). Opaque structs (time.Time, types
// implementing bson.Marshaler / bson.ValueMarshaler) are written as scalar leaves.
//
// # Usage
//
//	type User struct {
//	    ID         string    `bson:"_id"          behavior:"identifier"`
//	    Name       string    `bson:"name"`
//	    Password   string    `bson:"password"     behavior:"input_only"`
//	    CreateTime time.Time `bson:"create_time"  behavior:"output_only"`
//	}
//
//	ins := behavior.New[bson.M](mongo.NewInsertTranslator())
//	doc, err := ins.Translate(ctx, &user)
//
//	upd := behavior.New[bson.M](mongo.NewUpdateTranslator(),
//	    behavior.WithKinds(behavior.DefaultUpdateKinds...))
//	setUnset, err := upd.Translate(ctx, &user) // excludes _id, create_time
//
//	proj := behavior.New[bson.M](mongo.NewProjectionTranslator(),
//	    behavior.WithKinds(behavior.DefaultResponseKinds...), behavior.WithSchemaWalk())
//	p, err := proj.Translate(ctx, User{}) // {"password": 0}
package mongo
