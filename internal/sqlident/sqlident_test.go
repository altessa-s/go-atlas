// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqlident_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/sqlident"
)

func TestValidate(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", "t; DROP TABLE x", "a.b.c", "1tasks", `"quoted"`, "tab le", ".t", "s.", "a..b", "s.1t", "t\n", "t-x", "ü",
		strings.Repeat("t", sqlident.MaxIdentLen+1), "s." + strings.Repeat("t", sqlident.MaxIdentLen+1)} {
		err := sqlident.Validate(name)
		require.ErrorIs(t, err, sqlident.ErrInvalidTableName, name)
	}
	long := strings.Repeat("t", sqlident.MaxIdentLen)
	err := sqlident.Validate(long + "." + long)
	require.NoError(t, err, "the limit applies per part and is inclusive")
}
