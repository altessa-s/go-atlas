// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqlproject

import (
	"strings"

	"github.com/altessa-s/go-atlas/data/projection"
	"github.com/altessa-s/go-atlas/internal/sqldialect"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// columnSizeHint is the expected rendered size of one column: a quoted
// identifier of a dozen characters and a separator.
const columnSizeHint = 16

// NewContext builds the translator context for a SQL backend. SQL has no
// exclusion projection, so a policy whose empty request would resolve to
// "everything except the denied fields" is rejected with
// [projection.ErrDefaultFieldsRequired]: the caller must list the default
// columns with [projection.WithDefaultFields] or a restrictive allow-list.
// The default columns are rendered once in style, so a default that maps to
// a non-column fails here rather than on every empty request.
func NewContext(style sqldialect.Style, opts ...projection.TranslatorOption) (*projection.TranslatorContext, error) {
	ctx, err := projection.NewTranslatorContext(opts...)
	if err != nil {
		return nil, err
	}
	def := ctx.DefaultSelection()
	if len(def.Exclude) > 0 {
		return nil, coreerrs.Wrapf(projection.ErrDefaultFieldsRequired,
			"SQL cannot exclude denied columns %v", def.Exclude)
	}
	if _, err := render(style, def.Include); err != nil {
		return nil, err
	}
	return ctx, nil
}

// Translate resolves spec against cfg and renders the column list of a
// SELECT — `col, …` without the keyword — in the given quoting style.
// Columns appear in lexical order of their storage name, without aliases,
// so callers scan rows by column name (rows.Columns) rather than position.
// Every storage name must be a plain `col` or `table.col` identifier;
// anything else is rejected rather than spliced into the query. A selection
// of every field renders "", for which the caller writes `*`.
func Translate(cfg *projection.TranslatorContext, style sqldialect.Style, spec projection.Spec) (string, error) {
	sel, err := cfg.Resolve(spec)
	if err != nil {
		return "", err
	}
	return render(style, sel.Include)
}

// render quotes each column name and joins them with ", ".
func render(style sqldialect.Style, columns []string) (string, error) {
	var b strings.Builder
	b.Grow(len(columns) * columnSizeHint)
	for i, name := range columns {
		col, err := style.Table(name)
		if err != nil {
			return "", coreerrs.Wrapf(projection.ErrInvalidFieldPath, "%s is not a SQL column name: %v", name, err)
		}
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(col)
	}
	return b.String(), nil
}
