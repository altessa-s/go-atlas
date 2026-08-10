// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package toml_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	toml "github.com/altessa-s/go-atlas/config/loader/backend/toml"
)

// config is the destination a loader decodes into.
type config struct {
	Key   string `toml:"key"`
	Num   int    `toml:"num"`
	Value any    `toml:"value"`
}

// FuzzDecodeLeavesTheTargetUntouchedOnFailure pins the wrapper's contract: a
// document it refuses does not half-populate the destination.
//
// Configuration is decoded into a struct the process then runs on, and a loader
// that logs the error and carries on — or reuses the destination across a hot
// reload — would run on whatever the failed decode wrote. There is deliberately
// no round-trip target here: Decode delegates to the TOML library, so a value
// that does not survive re-encoding says something about that library's type
// mapping rather than about this package.
func FuzzDecodeLeavesTheTargetUntouchedOnFailure(f *testing.F) {
	f.Add([]byte(`key = "value"`))
	f.Add([]byte(""))
	f.Add([]byte("[[array]]\nid = 1"))
	f.Add([]byte(`nested.key = "value"`))
	f.Add([]byte("num = notanumber"))
	f.Add([]byte("key = "))
	f.Add([]byte(strings.Repeat("[a]\n", 64)))
	f.Add([]byte("\x00"))

	backend := &toml.Backend{}

	f.Fuzz(func(t *testing.T, data []byte) {
		sentinel := config{Key: "untouched", Num: -1}

		target := sentinel
		if err := backend.Decode(bytes.NewReader(data), &target); err != nil {
			require.Equal(t, sentinel, target,
				"a refused document left the destination modified: %q", data)
			return
		}

		// A successful decode must be repeatable: the loader reads the same
		// file again on a hot reload, and a document accepted once and refused
		// the next time swaps the running configuration for the previous one.
		// The second decode starts from the same state as the first: TOML leaves
		// fields the document does not mention untouched, so comparing against
		// a zero value would measure the fixture rather than the backend.
		again := sentinel
		require.NoError(t, backend.Decode(bytes.NewReader(data), &again),
			"the backend accepted a document once and refused it the next time: %q", data)
		require.Equal(t, target, again, "decoding the same document twice gave two values: %q", data)
	})
}
