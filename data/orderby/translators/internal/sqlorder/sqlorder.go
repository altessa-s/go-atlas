// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqlorder

import (
	"strings"

	"github.com/altessa-s/go-atlas/data/orderby"
	"github.com/altessa-s/go-atlas/internal/sqldialect"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Translate renders ob as the body of an ORDER BY clause — `col DIR, …`,
// without the keywords — for the given quoting style. Each key must be
// allowed by cfg; its mapped name must be a plain column or table.column
// identifier, which is quoted part by part. An empty spec renders "".
func Translate(cfg *orderby.TranslatorContext, style sqldialect.Style, ob orderby.Spec) (string, error) {
	if ob.IsEmpty() {
		return "", nil
	}
	var b strings.Builder
	for i, k := range ob.Keys {
		if !cfg.IsFieldAllowed(k.Name) {
			return "", coreerrs.Wrapf(orderby.ErrFieldNotAllowed, "%s", k.Name)
		}
		col, err := style.Table(cfg.ApplyFieldMapping(k.Name))
		if err != nil {
			// The column position cannot be bound as a parameter, so a
			// mapping to anything but an identifier — an expression, a JSON
			// path, a quote — is rejected rather than spliced in.
			return "", coreerrs.Wrapf(orderby.ErrInvalidFieldPath, "%s is not a SQL column name: %v", k.Name, err)
		}
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(col)
		if k.IsDescending() {
			b.WriteString(" DESC")
		} else {
			b.WriteString(" ASC")
		}
	}
	return b.String(), nil
}
