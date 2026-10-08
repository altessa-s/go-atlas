// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
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
//   - [NewInsertTranslator] builds an insert document. An insert applies no
//     behavior-kind filtering, so under the default bson tag the document is
//     the BSON codec's own encoding of the value, decoded into a bson.M
//     (nested documents as bson.D, arrays as bson.A, values as their BSON Go
//     types such as bson.DateTime and bson.Binary): exactly what the driver
//     stores. With a custom tag (WithBsonTagName) the codec cannot read the
//     names, and the document is folded field by field instead. Drive it with
//     no behavior.WithKinds.
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
//   - [NewFieldPathsTranslator] returns [FieldPaths]: the selectable and the
//     denied dot-notation paths of a model, ready for the
//     data/projection allow-list and WithDeniedStorageFields. Same engine
//     setup as the projection translator.
//
// # Document layout
//
// Documents and projection paths follow the layout the v2 BSON struct codec
// derives from each struct type: a `bson:",inline"` struct's fields, and an
// inline map's entries, sit at the parent level; the shallowest field of a
// name dominates, so an own field shadows an inlined one even when omitted or
// stripped; same-depth duplicates and inline map keys naming a field are
// errors, decided by the type. An embedded struct without inline is a
// subdocument named after its type, as the codec stores it.
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
