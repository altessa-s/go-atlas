// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisearch

import (
	"github.com/altessa-s/go-atlas/data/orderby"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// SortBy is the single-field sort instruction RediSearch's
// FT.SEARCH ... SORTBY clause expects. A zero-value SortBy (Field == "")
// signals that no SORTBY clause should be appended.
type SortBy struct {
	Field      string
	Descending bool
}

// Translator converts an [orderby.Spec] into a single-field RediSearch
// sort instruction. It is safe for concurrent use after construction.
type Translator struct {
	config *orderby.TranslatorContext
}

// NewTranslator creates a new RediSearch sort translator with the given
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

// Translate converts an [orderby.Spec] into a [SortBy]. Returns a
// zero-value SortBy for an empty input and [orderby.ErrTooManySortKeys]
// when more than one key is present — RediSearch's SORTBY accepts a
// single field only.
func (t *Translator) Translate(ob orderby.Spec) (SortBy, error) {
	if ob.IsEmpty() {
		return SortBy{}, nil
	}
	if len(ob.Keys) > 1 {
		return SortBy{}, coreerrs.Wrapf(orderby.ErrTooManySortKeys,
			"redisearch supports a single SORTBY field, got %d", len(ob.Keys))
	}

	k := ob.Keys[0]
	if !t.config.IsFieldAllowed(k.Name) {
		return SortBy{}, coreerrs.Wrapf(orderby.ErrFieldNotAllowed, "%s", k.Name)
	}

	return SortBy{
		Field:      t.config.ApplyFieldMapping(k.Name),
		Descending: k.IsDescending(),
	}, nil
}
