// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/config/loader/secrets"
)

func FuzzHasSecrets(f *testing.F) {
	f.Add("")
	f.Add("plain text")
	f.Add("$__secret{ns:key}")
	f.Add("$__secret{}")
	f.Add("$__secret{no_colon}")
	f.Add("$$__secret{ns:key}")
	f.Add("$__secret{a:b}$__secret{c:d}")

	f.Fuzz(func(t *testing.T, content string) {
		_ = secrets.HasSecrets(content)
	})
}

func FuzzExpander_Expand(f *testing.F) {
	f.Add("")
	f.Add("plain")
	f.Add("$__secret{ns:key}")
	f.Add("pre $__secret{a:b} post")
	f.Add("$__secret{a:b}$__secret{c:d}")
	f.Add("${not_a_secret}")

	mgr := newMockManager(map[string]string{
		"ns:key": "value",
		"a:b":    "val1",
		"c:d":    "val2",
	})
	expander := secrets.New(mgr)
	ctx := f.Context()

	f.Fuzz(func(t *testing.T, content string) {
		// Should not panic
		_, _ = expander.Expand(ctx, content)
	})
}
