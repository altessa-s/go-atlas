// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package rules_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oidc/rules"
)

func BenchmarkEvaluateCEL(b *testing.B) {
	matches, err := rules.CompileCEL("claims.iat > 0.0")
	require.NoError(b, err)
	claims := map[string]any{"iat": json.Number("1750000000.000000001")}
	for b.Loop() {
		matches(claims)
	}
}
