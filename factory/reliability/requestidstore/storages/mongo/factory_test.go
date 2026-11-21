// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	corefactory "github.com/altessa-s/go-atlas/factory/core"
	factory "github.com/altessa-s/go-atlas/factory/reliability/requestidstore/storages/mongo"
)

func TestMissingDependency(t *testing.T) {
	t.Parallel()
	_, err := factory.Builder(nil)(t.Context(), nil)
	require.ErrorIs(t, err, corefactory.ErrDependencyRequired)
}
