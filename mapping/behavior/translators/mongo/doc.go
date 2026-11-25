// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongo provides [github.com/altessa-s/go-atlas/mapping/behavior.Translator]
// implementations that fold behavior-tagged Go structs into MongoDB BSON
// documents from a single behavior reflection walk.
//
// The store binds its CSFLE [FieldTransformer] to the insert and update
// translators using [WithFieldTransformer]. Each translation supplies its own
// context and caches field selections for that call. Drive translators through
// [github.com/altessa-s/go-atlas/mapping/behavior.New], which owns the behavior
// tag name, traversal depth, and strip-kind policy.
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
//     "_id" is removed from both operators, and any field the engine marked Strip is
//     excluded from both operators. A nested struct holding such a protected
//     field (stripped, identifier or omitonupdate) is written by descendant
//     paths ("profile.name") so the stored value survives; a replaced
//     collection whose elements carry a stripped field, or a protected
//     omitonupdate one, fails with [ErrProtectedInCollection]. A nil pointer,
//     a nil or empty collection or a nil element whose declared type holds a
//     protected field — under the engine's tag key and strip kinds, bound by
//     behavior.New — fails with [ErrProtectedUnset] or
//     [ErrProtectedInCollection] rather than delete the stored value (see
//     [NewUpdateTranslator]). Identifier fields are excluded by the
//     translator itself, whatever kinds the engine strips: the identifier
//     selects the record and is never written. Drive it with
//     behavior.WithKinds(behavior.DefaultUpdateKinds...).
//   - [NewProjectionTranslator] builds an exclusion projection ({path: 0}) for
//     every stripped field, using dot-notation paths for nested fields. It is
//     type-driven and requests a schema walk from the engine automatically;
//     drive it with behavior.WithKinds(behavior.DefaultResponseKinds...).
//   - [NewFieldPathsTranslator] returns [FieldPaths]: the selectable and the
//     denied dot-notation paths of a model, ready for the
//     go-aip projection allow-list and WithDeniedStorageFields. Same engine
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
//	    behavior.WithKinds(behavior.DefaultResponseKinds...))
//	p, err := proj.Translate(ctx, User{}) // {"password": 0}
package mongo
