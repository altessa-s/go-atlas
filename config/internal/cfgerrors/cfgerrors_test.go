// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cfgerrors_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/internal/cfgerrors"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

func TestInvalidf(t *testing.T) {
	t.Parallel()

	cause := errors.New("root cause")
	err := cfgerrors.Invalidf("field %q: %w", "hosts", cause)

	require.Equal(t, `field "hosts": root cause`, err.Error())
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	require.ErrorIs(t, err, cause)
}

func TestInvalidf_SurvivesJoin(t *testing.T) {
	t.Parallel()

	err := errors.Join(cfgerrors.Invalidf("a"), cfgerrors.Invalidf("b"))

	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
}
