// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mariadb

import (
	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/filter/translators/internal/sqlbase"
)

// Translator converts filter AST nodes to MariaDB SQL WHERE clauses.
//
// Translate emits `?` placeholders and returns the bound values
// alongside the clause; TranslateInline emits self-contained SQL with
// every literal rendered in place. Both are promoted from the shared SQL
// walker — see [sqlbase.Translator].
//
// A Translator carries per-call state and is NOT safe for concurrent
// use. Construct one per goroutine, or guard it with a mutex.
type Translator struct {
	*sqlbase.Translator
}

// NewTranslator creates a new MariaDB translator with the given options.
// Returns [filter.ErrAllowlistRequired] when [filter.WithUntrustedInput]
// is set without a non-empty [filter.WithAllowedFields] — the
// misconfiguration is surfaced here rather than on the first Translate
// call.
func NewTranslator(opts ...filter.TranslatorOption) (*Translator, error) {
	base, err := sqlbase.New(dialect{}, opts...)
	if err != nil {
		return nil, err
	}
	return &Translator{Translator: base}, nil
}
