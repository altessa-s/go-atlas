// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package yaml3_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	yaml3 "github.com/altessa-s/go-atlas/config/loader/backend/yaml3"
)

// secretContent is written to a file OUTSIDE the include root. Any appearance
// of it in preprocessed output means a directive escaped the root.
const secretContent = "THIS-IS-OUTSIDE-THE-ROOT"

// FuzzPreprocessNeverReadsOutsideTheRoot is the include-traversal oracle.
//
// `!include` inlines a file into the configuration, and the root directory is
// the only thing stopping a directive from naming one that was never meant to
// be configuration. The guard is genuinely intricate — an extension check, a
// symlink resolution, and a filepath.Rel containment test that must agree about
// which path they are talking about — and a config file is not always fully
// trusted: it is templated, assembled by a deploy tool, or edited by someone
// with narrower access than the process has.
//
// The check is not on the returned error but on the returned text: whatever
// happens, the bytes of a file outside the root must never appear in it.
func FuzzPreprocessNeverReadsOutsideTheRoot(f *testing.F) {
	f.Add("!include ../outside.yaml")
	f.Add("!include ./inside.yaml")
	f.Add("!include /etc/passwd")
	f.Add("!include ../../outside.yaml")
	f.Add("  !include ../outside.yaml")
	f.Add("!include outside.yaml/../../outside.yaml")
	f.Add("key: value")
	f.Add("")
	f.Add("!include link.yaml")

	f.Fuzz(func(t *testing.T, content string) {
		base := t.TempDir()
		root := filepath.Join(base, "root")
		require.NoError(t, os.MkdirAll(root, 0o750))

		// A readable YAML file the directive is allowed to reach.
		require.NoError(t, os.WriteFile(filepath.Join(root, "inside.yaml"),
			[]byte("inside: ok\n"), 0o600))

		// The same, one level up — outside the root, and therefore off limits.
		outside := filepath.Join(base, "outside.yaml")
		require.NoError(t, os.WriteFile(outside, []byte("secret: "+secretContent+"\n"), 0o600))

		// A symlink inside the root pointing at it, which is the case the
		// resolution step exists for.
		_ = os.Symlink(outside, filepath.Join(root, "link.yaml"))

		backend := &yaml3.Backend{}
		out, err := backend.Preprocess(content, root, root)
		if err != nil {
			require.Empty(t, out, "a rejected document must not also produce output")
			return
		}

		require.NotContains(t, out, secretContent,
			"an include reached outside the root:\n%s", out)
	})
}

// FuzzDecodeLeavesTheTargetUntouchedOnFailure pins the wrapper's own contract:
// a document it refuses does not half-populate the destination.
//
// Configuration is decoded into a struct the process then runs on, and a loader
// that logs a decode error and carries on — or that reuses the destination
// across a hot reload — would run on whatever the failed decode managed to
// write. Note the deliberate absence of a round-trip target here: Decode
// delegates straight to yaml.v3, so a value that does not survive re-encoding
// says something about the library's number typing rather than about this
// package.
func FuzzDecodeLeavesTheTargetUntouchedOnFailure(f *testing.F) {
	f.Add([]byte("key: value"))
	f.Add([]byte(""))
	f.Add([]byte("a: [1, 2, 3]"))
	f.Add([]byte("a: &x 1\nb: *x"))
	f.Add([]byte("0: [08]"))
	f.Add([]byte("!"))
	f.Add([]byte("\x00"))
	f.Add([]byte(strings.Repeat("a: &a [*a]\n", 4)))

	backend := &yaml3.Backend{}

	f.Fuzz(func(t *testing.T, data []byte) {
		sentinel := map[string]any{"untouched": true}

		target := map[string]any{"untouched": true}
		if err := backend.Decode(bytes.NewReader(data), &target); err != nil {
			require.Equal(t, sentinel, target,
				"a refused document left the destination modified: %q", data)
			return
		}

		// A successful decode must at least be repeatable against the same
		// input — the loader reads the file more than once on a hot reload.
		var again map[string]any
		require.NoError(t, backend.Decode(bytes.NewReader(data), &again),
			"the backend accepted a document once and refused it the next time: %q", data)
	})
}
