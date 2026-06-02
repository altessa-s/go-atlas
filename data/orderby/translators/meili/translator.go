// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meili

import (
	"github.com/altessa-s/go-atlas/data/orderby"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Translator converts an [orderby.Spec] into the Meilisearch `sort`
// argument: a `[]string` of "field:asc" / "field:desc" entries. It is
// safe for concurrent use after construction.
type Translator struct {
	config *orderby.TranslatorContext
}

// NewTranslator creates a new Meilisearch sort translator with the given
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

// Translate converts an [orderby.Spec] into the Meilisearch sort
// argument. Returns a nil slice for an empty Spec so callers can omit
// the `sort` parameter entirely.
func (t *Translator) Translate(ob orderby.Spec) ([]string, error) {
	if ob.IsEmpty() {
		return nil, nil
	}

	out := make([]string, 0, len(ob.Keys))
	for _, k := range ob.Keys {
		if !t.config.IsFieldAllowed(k.Name) {
			return nil, coreerrs.Wrapf(orderby.ErrFieldNotAllowed, "%s", k.Name)
		}
		direction := "asc"
		if k.IsDescending() {
			direction = "desc"
		}
		out = append(out, t.config.ApplyFieldMapping(k.Name)+":"+direction)
	}
	return out, nil
}
