// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"github.com/altessa-s/go-atlas/data/projection"
	"github.com/altessa-s/go-atlas/data/projection/translators/internal/sqlproject"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
)

// Translator converts a [projection.Spec] into the column list of a PostgreSQL
// SELECT. It is safe for concurrent use after construction.
type Translator struct {
	config *projection.TranslatorContext
}

// NewTranslator creates a PostgreSQL projection translator. Beyond the
// construction errors of [projection.NewTranslatorContext], it returns
// [projection.ErrDefaultFieldsRequired] when an empty request would have to
// exclude denied columns, which SQL cannot express.
func NewTranslator(opts ...projection.TranslatorOption) (*Translator, error) {
	ctx, err := sqlproject.NewContext(sqldialect.Postgres, opts...)
	if err != nil {
		return nil, err
	}
	return &Translator{config: ctx}, nil
}

// Translate resolves spec against the policy and renders the quoted column
// list, without the SELECT keyword, in lexical order of the column names.
// A selection of every column renders "", for which the caller writes `*`.
func (t *Translator) Translate(spec projection.Spec) (string, error) {
	return sqlproject.Translate(t.config, sqldialect.Postgres, spec)
}
