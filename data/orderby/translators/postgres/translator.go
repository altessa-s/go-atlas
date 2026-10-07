// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"github.com/altessa-s/go-atlas/data/orderby"
	"github.com/altessa-s/go-atlas/data/orderby/translators/internal/sqlorder"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
)

// Translator converts an [orderby.Spec] into a PostgreSQL ORDER BY body. It
// is safe for concurrent use after construction.
type Translator struct {
	config *orderby.TranslatorContext
}

// NewTranslator creates a new PostgreSQL sort translator with the given
// options. Returns [orderby.ErrAllowlistRequired] when
// [orderby.WithUntrustedInput] is set without a non-empty
// [orderby.WithAllowedFields].
func NewTranslator(opts ...orderby.TranslatorOption) (*Translator, error) {
	ctx, err := orderby.NewTranslatorContext(opts...)
	if err != nil {
		return nil, err
	}
	return &Translator{config: ctx}, nil
}

// Translate converts an [orderby.Spec] into the body of an ORDER BY clause,
// for example `"created_at" DESC, "slug" ASC`. An empty Spec translates to "".
func (t *Translator) Translate(ob orderby.Spec) (string, error) {
	return sqlorder.Translate(t.config, sqldialect.Postgres, ob)
}
