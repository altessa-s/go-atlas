// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/secrets"
)

func BenchmarkManager_RunUpdateCycle(b *testing.B) {
	values := make(map[string]string, 500)
	for i := range 500 {
		values[fmt.Sprintf("key-%d", i)] = "value"
	}
	provider := &versionedProvider{values: values}
	mgr, err := secrets.New[string](provider)
	require.NoError(b, err)
	ctx := b.Context()

	for b.Loop() {
		if err := mgr.RunUpdateCycle(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
