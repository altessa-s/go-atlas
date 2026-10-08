// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sqlproject renders a resolved projection as the column list of a
// SQL SELECT for the postgres, mariadb and clickhouse translators, which
// differ only in their identifier quoting style.
//
//	ctx, err := sqlproject.NewContext(sqldialect.Postgres, opts...)
//	cols, err := sqlproject.Translate(ctx, sqldialect.Postgres, spec)
//	// cols == `"id", "name"`
package sqlproject
