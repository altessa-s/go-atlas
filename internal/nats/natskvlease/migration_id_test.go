// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// newMigrationID is embedded in template bucket names and lease owners, so
// its format is pinned: 2*migrationIDBytes lowercase hex digits, random per call.
func TestNewMigrationID(t *testing.T) {
	t.Parallel()

	format := regexp.MustCompile(`^[0-9a-f]{16}$`)
	seen := make(map[string]struct{})
	for range 64 {
		id := newMigrationID()
		require.Len(t, id, 2*migrationIDBytes)
		require.Regexp(t, format, id)
		require.NotContains(t, seen, id)
		seen[id] = struct{}{}
	}
}
