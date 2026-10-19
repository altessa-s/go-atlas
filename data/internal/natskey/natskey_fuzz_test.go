// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskey_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/internal/natskey"
)

// FuzzEncode checks that every encoding is a key the server accepts, that it
// decodes back to the input (which makes Encode injective), and that a key
// needing no escaping is kept as is.
func FuzzEncode(f *testing.F) {
	for _, seed := range []string{
		"", "=", "==", "=3A", "a=3Ab", "a:b", ".", "..", "a..b", ".a.", "a/b//c",
		"a*b>c", "a b", "\x00", "κλειδί.ü", "\xff\xfe", "idk:POST:/orders:123", "2001:db8::1",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, key string) {
		physical, err := natskey.Encode(key)
		if errors.Is(err, natskey.ErrKeyTooLong) {
			require.Greater(t, len(key), natskey.MaxLen/3)
			return
		}
		require.NoError(t, err)
		require.LessOrEqual(t, len(physical), natskey.MaxLen)
		requireServerAccepts(t, physical)

		back, err := natskey.Decode(physical)
		require.NoError(t, err)
		require.Equal(t, key, back)

		if key != "" && keyPattern.MatchString(key) && physical != key {
			// Only '=' and dots that would leave an empty token may change a
			// key that already matches the character set.
			require.Regexp(t, `=|^\.|\.$|\.\.`, key)
		}
	})
}
