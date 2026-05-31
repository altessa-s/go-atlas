// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/data/orderby"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ascending and descending are the int32 values MongoDB expects in a sort
// document for the two directions.
const (
	ascending  int32 = 1
	descending int32 = -1
)

// Translator converts an [orderby.Spec] into a MongoDB [bson.D] sort
// document. It is safe for concurrent use after construction.
type Translator struct {
	config *orderby.TranslatorContext
}

// NewTranslator creates a new MongoDB sort translator with the given
// options. Returns [orderby.ErrAllowlistRequired] when
// [orderby.WithUntrustedInput] is set without a non-empty
// [orderby.WithAllowedFields] — the misconfiguration is surfaced here
// rather than on the first Translate call.
func NewTranslator(opts ...orderby.TranslatorOption) (*Translator, error) {
	ctx, err := orderby.NewTranslatorContext(opts...)
	if err != nil {
		return nil, err
	}
	return &Translator{config: ctx}, nil
}

// Translate converts an [orderby.Spec] into a MongoDB sort document.
// An empty Spec translates to a nil bson.D with no error — matching
// the Meilisearch translator's "nil for empty" shape so call sites can
// use the same `if sort != nil` check across backends.
// `SetSort(nil)` is valid in the MongoDB driver and is equivalent to
// omitting the sort entirely.
func (t *Translator) Translate(ob orderby.Spec) (bson.D, error) {
	if ob.IsEmpty() {
		return nil, nil
	}
	out := make(bson.D, 0, len(ob.Keys))
	for _, k := range ob.Keys {
		if !t.config.IsFieldAllowed(k.Name) {
			return nil, coreerrs.Wrapf(orderby.ErrFieldNotAllowed, "%s", k.Name)
		}
		value := ascending
		if k.IsDescending() {
			value = descending
		}
		out = append(out, bson.E{
			Key:   t.config.ApplyFieldMapping(k.Name),
			Value: value,
		})
	}
	return out, nil
}
