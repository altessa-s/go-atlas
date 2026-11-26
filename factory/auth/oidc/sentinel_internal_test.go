// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	"github.com/stretchr/testify/require"

	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

func TestHelpers_NilConfigReturnsSentinel(t *testing.T) {
	t.Parallel()

	_, err := validationOptionsFromConfig(nil, 0)
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)

	_, err = presetFromConfig(nil, 0)
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)
}
