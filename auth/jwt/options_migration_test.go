// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptionsMigrationBaseline(t *testing.T) {
	t.Parallel()
	t.Run("allowed algorithms", func(t *testing.T) {
		t.Parallel()
		before := []Algorithm{Algorithm(" HS256 "), Algorithm("HS256"), Algorithm("HS256")}
		got := defaultVerifierOptions()
		WithVerifierAllowedAlgorithms()(got)
		require.Equal(t, DefaultAllowedAlgorithms(), got.allowedAlgorithms)
		WithVerifierAllowedAlgorithms(before...)(got)
		before[0] = AlgRS256
		require.Equal(t, []Algorithm{" HS256 ", "HS256", "HS256"}, got.allowedAlgorithms)
	})
	t.Run("audiences", func(t *testing.T) {
		t.Parallel()
		got := defaultVerifierOptions()
		got.audiences = []string{"existing"}
		WithAudiences()(got)
		require.Equal(t, []string{"existing"}, got.audiences)
		input := []string{" api ", "api"}
		WithAudiences(input...)(got)
		input[0] = "changed"
		require.Equal(t, []string{" api ", "api"}, got.audiences)
	})
	t.Run("required claims", func(t *testing.T) {
		t.Parallel()
		got := defaultVerifierOptions()
		got.requiredClaims = []string{"existing"}
		WithRequiredClaims()(got)
		require.Equal(t, []string{"existing"}, got.requiredClaims)
		input := []string{" custom ", "custom"}
		WithRequiredClaims(input...)(got)
		input[0] = "changed"
		require.Equal(t, []string{" custom ", "custom"}, got.requiredClaims)
	})
}
