// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/mapping/behavior"
)

type updateTranslator struct{ o *options }

// NewUpdateTranslator returns a [behavior.Translator] producing update documents
// ({"$set": …, "$unset": …}) under opts. "_id" is removed from both; nil
// pointers and empty collections of a non-stripped field go to "$unset"; nested
// values are full-replaced in "$set", except a nested object holding a field
// the update leaves out, which is written by descendant paths ("profile.name")
// so the stored value of that field survives. Fields the engine marked Strip
// are excluded from both operators — recursively through nested objects; a
// replaced collection whose elements carry one fails with
// [ErrProtectedInCollection], as the replacement cannot preserve it. A value
// that would remove stored data without resolved fields to check — a nil
// pointer, a nil or empty collection, a nil element — is judged from its
// declared type under the engine's own policy, which [behavior.New] binds to
// the translator (see [behavior.PolicyBinder]): when the type holds a field of
// a kind the engine strips, read under the engine's tag key (or, outside a
// collection, an identifier or omitonupdate field), the update fails with
// [ErrProtectedUnset] for a subdocument and [ErrProtectedInCollection] for a
// collection or inside one, instead of deleting the stored value. Populated
// and removed values thus follow the same policy.
// Drive it through
// [github.com/altessa-s/go-atlas/mapping/behavior.New] with
// behavior.WithKinds(behavior.DefaultUpdateKinds...) to exclude server-owned and
// immutable fields. Identifier fields are excluded by the translator itself,
// whatever kinds the engine strips: the identifier selects the record and is
// never written, under "_id" or any other name, set or nil. An identifier
// inside a collection element is the element's own and is written with it;
// stripped by the engine, only when it carries no other kind and its value is
// written as a unit (see excludeOnUpdate).
func NewUpdateTranslator(opts ...Option) behavior.Translator[bson.M] {
	return &updateTranslator{o: newOptions(opts...)}
}

// BindPolicy implements [behavior.PolicyBinder]: [behavior.New] calls it with
// the engine's policy, which the update translator applies to the values it
// judges from their declared type. t itself is left unchanged.
func (t *updateTranslator) BindPolicy(p behavior.Policy) behavior.Translator[bson.M] {
	o := *t.o
	o.policy = p
	return &updateTranslator{o: &o}
}

func (t *updateTranslator) Translate(ctx context.Context, root behavior.Object) (bson.M, error) {
	return t.o.translateDocument(ctx, root, t.encode)
}

func (t *updateTranslator) encode(root behavior.Object, ec encCtx) (bson.M, error) {
	// The root's keys are operator paths: a field whose stored name holds
	// a dot (bson:"a.b", a literal key to the codec) would address another
	// field, and a "$" prefix is not a field at all, so folding checks
	// each one (see encCtx.addressed).
	ec.addressed, ec.root = true, true
	set, unset, err := foldObject(root, t.o, modeUpdate, ec)
	if err != nil {
		return nil, err
	}
	// _id is immutable in MongoDB: neither operator may touch it. Folding
	// skips the root _id field (own or inline-contributed, with every
	// descendant path); an inline map entry named _id is dropped here.
	delete(set, "_id")
	delete(unset, "_id")
	result := bson.M{"$set": set}
	if len(unset) > 0 {
		result["$unset"] = unset
	}
	return result, nil
}
