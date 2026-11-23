// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package diagnostics

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrorTextRedactsBeforeTruncation(t *testing.T) {
	secret := strings.Repeat("p", 2000)
	raw := "dial\r\n postgres://user%40example:" + secret + "@[::1]:5432/db or https://token@localhost/test"
	const safe = "dial postgres://[::1]:5432/db or https://localhost/test"
	require.Equal(t, safe, ErrorText(raw, DefaultMaxErrorLength))
	require.Equal(t, safe[:21]+"...", ErrorText(raw, 26))
}
