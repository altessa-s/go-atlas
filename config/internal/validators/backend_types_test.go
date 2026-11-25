// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validators_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/internal/validators"
)

func TestBackendTypesSnapshot(t *testing.T) {
	t.Parallel()
	var names validators.BackendTypes[string]
	extra := []string{"custom"}
	names.SetRegisteredTypes(extra)
	extra[0] = "changed"
	snapshot := names
	names.SetRegisteredTypes(nil)
	require.Equal(t, []string{"memory"}, names.AllowedTypes("memory"))
	allowed := snapshot.AllowedTypes("memory")
	require.Equal(t, []string{"memory", "custom"}, allowed)
	allowed[1] = "changed"
	require.Equal(t, []string{"custom"}, snapshot.AllowedTypes())
}
