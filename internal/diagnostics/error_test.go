// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package diagnostics

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestErrorTextRedactsBeforeTruncation(t *testing.T) {
	secret := strings.Repeat("p", 2000)
	raw := "dial\r\n postgres://user%40example:" + secret + "@[::1]:5432/db or https://token@localhost/test"
	const safe = "dial postgres://[::1]:5432/db or https://localhost/test"
	require.Equal(t, safe, ErrorText(raw, DefaultMaxErrorLength))
	require.Equal(t, safe[:23]+"...", ErrorText(raw, 26))
}

func TestErrorTextRedactsCredentialsAcrossControlCharacters(t *testing.T) {
	require.Equal(t, "dial https://db/atlas", ErrorText("dial https://user:pass\r\nword@db/atlas", DefaultMaxErrorLength))
}

func TestSanitizeBinaryError(t *testing.T) {
	const limit = 20
	got := ErrorText("\n"+strings.Repeat("\xffx", limit), limit)
	require.True(t, utf8.ValidString(got))
	require.LessOrEqual(t, len(got), limit)
}

func TestErrorTextRedactsAfterRemovingMalformedBytesAndControls(t *testing.T) {
	const raw = "dial https:/\xff/worker:pa\r\nss@db/atlas"
	const safe = "dial https://db/atlas"
	require.Equal(t, safe, ErrorText(raw, len(safe)))
}
