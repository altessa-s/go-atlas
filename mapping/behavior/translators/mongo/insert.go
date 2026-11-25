// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"fmt"
	"reflect"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/mapping/behavior"
)

type insertTranslator struct{ o *options }

// NewInsertTranslator returns a [behavior.Translator] producing insert documents
// under opts. It emits every exported field (bson name, omitempty, []byte binary,
// nested/opaque-leaf as today) and applies no behavior-kind filtering — drive it
// through [github.com/altessa-s/go-atlas/mapping/behavior.New] with no
// behavior.WithKinds so nothing is marked Strip.
func NewInsertTranslator(opts ...Option) behavior.Translator[bson.M] {
	return &insertTranslator{o: newOptions(opts...)}
}

// BindPolicy implements [behavior.PolicyBinder]: [behavior.New] calls it with
// the engine's policy. The original translator remains reusable.
func (t *insertTranslator) BindPolicy(p behavior.Policy) behavior.Translator[bson.M] {
	o := *t.o
	o.policy = p
	return &insertTranslator{o: &o}
}

func (t *insertTranslator) Translate(ctx context.Context, root behavior.Object) (bson.M, error) {
	return t.o.translateDocument(ctx, root, t.encode)
}

func (t *insertTranslator) encode(root behavior.Object, ec encCtx) (bson.M, error) {
	if t.o.bsonTagName == DefaultBSONTagName && root.Value.IsValid() {
		if ec.transforms == nil {
			return encodeInsert(root.Value, t.o.registry)
		}
		// A root codec can choose arbitrary names or duplicate plaintext.
		if isCodecLeaf(root.Value, ec) || (root.Value.CanAddr() && isCodecLeaf(root.Value.Addr(), ec)) {
			return nil, ec.transforms.opaque(root.Value)
		}
	}
	set, _, err := foldObject(root, t.o, modeInsert, ec)
	if err != nil {
		return nil, err
	}
	if t.o.bsonTagName == DefaultBSONTagName && ec.transforms != nil {
		return encodeInsert(reflect.ValueOf(set), nil)
	}
	return set, nil
}

// encodeInsert builds an insert document by encoding v with the BSON codec
// itself — an insert applies no behavior filtering, so the document is
// exactly what the driver would store for v, inline fields, omitempty,
// minsize and marshalers included — and decoding it into a bson.M (nested
// documents as ordered bson.D, arrays as bson.A, values as their BSON Go
// types). v is
// encoded through its address when it has one, as the driver encodes a
// pointer, with reg when it is non-nil. Keys starting with "$" are rejected at
// any depth, as on the folding path, so no map can smuggle an operator-like key
// into the document.
func encodeInsert(v reflect.Value, reg *bson.Registry) (bson.M, error) {
	// The codec recurses forever on a struct inlined into itself; reject what
	// layoutOf rejects before handing the value over.
	if err := validateCodecInput(v); err != nil {
		return nil, err
	}
	target := v.Interface()
	if v.CanAddr() {
		target = v.Addr().Interface()
	}
	raw, err := marshal(target, reg)
	if err != nil {
		return nil, fmt.Errorf("behavior/mongo: encode insert document: %w", err)
	}
	if err := rejectOperatorKeys(raw); err != nil {
		return nil, err
	}
	// Nested documents decode as bson.D: they keep the codec's key order (and
	// any repeated key of an encoded bson.D), as the driver would store them.
	var out bson.M
	if err := bson.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("behavior/mongo: decode insert document: %w", err)
	}
	return out, nil
}
