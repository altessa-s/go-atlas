// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filesystem_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa/sources/filesystem"
)

// FuzzNewAcceptsOnlyDirectoriesAndRegoFiles pins what may become a policy
// source.
//
// The source feeds OPA the rules that authorize requests, so what it is willing
// to load from is an authorization decision of its own. The documented rule is
// narrow — an existing directory, or an existing file whose name ends in .rego
// — and the narrowness is the point: a source that accepted an arbitrary file
// would let whatever wrote that file choose the policy.
func FuzzNewAcceptsOnlyDirectoriesAndRegoFiles(f *testing.F) {
	f.Add("dir", true)
	f.Add("policy.rego", false)
	f.Add("policy.txt", false)
	f.Add("policy.rego.bak", false)
	f.Add("", false)
	f.Add(".", true)
	f.Add("..", false)
	f.Add("missing.rego", false)
	f.Add("REGO.REGO", false)

	f.Fuzz(func(t *testing.T, name string, asDir bool) {
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
			// A separator or a dot segment resolves out of the fixture, so the
			// path the source sees is not the one this target created.
			t.Skip("the name would not stay inside the fixture directory")
		}

		base := t.TempDir()
		path := filepath.Join(base, name)

		// Create the target so the existence check has something to find; a
		// fuzzer that only ever probed missing paths would exercise one branch.
		var created bool
		if asDir {
			created = os.MkdirAll(path, 0o750) == nil
		} else {
			created = os.WriteFile(path, []byte("package p\n"), 0o600) == nil
		}

		source, err := filesystem.New(path)
		if err != nil {
			require.Nil(t, source, "a rejected path must not also produce a source")
			return
		}
		t.Cleanup(func() { _ = source.Close() })

		require.True(t, created, "a path that was never created was accepted: %q", path)
		require.True(t, asDir || strings.HasSuffix(path, ".rego"),
			"a file that is not a .rego policy was accepted as a source: %q", path)
	})
}
