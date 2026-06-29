// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestWithValidationLeewayAllowsZero verifies WithValidationLeeway(0) takes
// effect (strict, no-skew validation) even when overriding a non-zero leeway
// set earlier in the option chain — it must not be silently dropped.
func TestWithValidationLeewayAllowsZero(t *testing.T) {
	t.Parallel()

	o := applyValidationOptionsToVerifier([]ValidationOption{
		WithValidationLeeway(30 * time.Second),
		WithValidationLeeway(0),
	})
	require.Zero(t, o.leeway)

	// A negative leeway is still rejected, leaving the prior value intact.
	o = applyValidationOptionsToVerifier([]ValidationOption{
		WithValidationLeeway(30 * time.Second),
		WithValidationLeeway(-time.Second),
	})
	require.Equal(t, 30*time.Second, o.leeway)
}
