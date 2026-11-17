// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validators

import (
	"strings"

	"github.com/altessa-s/go-atlas/config/internal/cfgerrors"
	"github.com/altessa-s/go-atlas/internal/sqldialect"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// MaxSQLKeyBatch caps the purge batch of the SQL storages whose MySQL sweep
// deletes selected keys through an IN list; see [sqldialect.MaxKeyBatch].
const MaxSQLKeyBatch = sqldialect.MaxKeyBatch

// SQLTableName validates a plain SQL identifier, optionally qualified by one
// schema name. Each part matches [A-Za-z_][A-Za-z0-9_]* and is at most 63
// characters. Empty strings pass; combine with validation.Required as needed.
var SQLTableName = validation.By(func(value any) error {
	value, isNil := validation.Indirect(value)
	if isNil {
		return nil
	}
	name, err := validation.EnsureString(value)
	if err != nil {
		return cfgerrors.Invalidf("SQL table name: %w", err)
	}
	if name == "" {
		return nil
	}
	if _, err := sqldialect.Postgres.Table(name); err != nil {
		return cfgerrors.Invalidf("SQL table name: %w", err)
	}
	return nil
})

// SQLIdentifier validates an unqualified SQL identifier matching
// [A-Za-z_][A-Za-z0-9_]* with at most 63 characters. Empty strings pass;
// combine with validation.Required as needed.
var SQLIdentifier = validation.By(func(value any) error {
	if err := SQLTableName.Validate(value); err != nil {
		return err
	}
	value, _ = validation.Indirect(value)
	name, _ := validation.EnsureString(value)
	if strings.Contains(name, ".") {
		return cfgerrors.Invalidf("SQL identifier %q must be unqualified: %w", name, sqldialect.ErrInvalidTableName)
	}
	return nil
})
