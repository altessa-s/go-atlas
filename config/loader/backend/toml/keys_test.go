// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package toml_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader/backend"
	"github.com/altessa-s/go-atlas/config/loader/backend/toml"
)

type KeysFlat struct {
	Port int `toml:"port"`
}

type KeysNamed struct {
	Host string `toml:"host"`
}

type KeysSkipped struct {
	Mode string `toml:"mode"`
}

type keysConfig struct {
	KeysFlat                        // flattened
	KeysNamed   `toml:"named"`      // keyed by its tag name
	KeysSkipped `toml:"-"`          // excluded
	Foo         struct{ X int }     // untagged: exact "Foo" first, then any casing
	FOO         struct{ Y int }     // untagged: exact "FOO"
	Items       []struct{ Z int }   `toml:"items"`
	ByName      map[string]KeysFlat `toml:"byName"`
}

var keysConfigType = reflect.TypeFor[keysConfig]()

func decodeKeys(t *testing.T, doc string) map[int]backend.KeyNode {
	t.Helper()
	root, err := (&toml.Backend{}).DecodeKeys(strings.NewReader(doc))
	require.NoError(t, err)
	require.False(t, root.IsNull())
	fs, ok := root.Fields(keysConfigType)
	require.True(t, ok)
	return fs
}

// Fields bind as BurntSushi/toml binds them: anonymous structs without a tag
// name are flattened, a tag name keys them, "-" excludes them.
func TestBackend_DecodeKeys_Embedding(t *testing.T) {
	t.Parallel()

	fs := decodeKeys(t, "port = 0\nmode = \"x\"\n[named]\nhost = \"\"\n")

	flat, ok := fs[0].Fields(reflect.TypeFor[KeysFlat]())
	require.True(t, ok)
	require.Contains(t, flat, 0)

	named, ok := fs[1].Fields(reflect.TypeFor[KeysNamed]())
	require.True(t, ok)
	require.Contains(t, named, 0)

	if skipped, ok := fs[2]; ok {
		inner, _ := skipped.Fields(reflect.TypeFor[KeysSkipped]())
		require.Empty(t, inner)
	}
}

// An exact field name wins over a case-insensitive match, and a key never
// binds to an absent sibling.
func TestBackend_DecodeKeys_CaseFolding(t *testing.T) {
	t.Parallel()

	fs := decodeKeys(t, "[Foo]\nX = 0\n")
	require.Contains(t, fs, 3)
	require.NotContains(t, fs, 4, "[Foo] binds to Foo exactly, not to FOO")

	fs = decodeKeys(t, "[foo]\nX = 0\n")
	require.Contains(t, fs, 3, "[foo] folds to the first case-insensitive match")
	require.NotContains(t, fs, 4)
}

// Map entries and array elements bind to their destination types.
func TestBackend_DecodeKeys_EntriesAndElems(t *testing.T) {
	t.Parallel()

	fs := decodeKeys(t, "[[items]]\nZ = 0\n[[items]]\n[byName.a]\nport = 0\n")

	items, ok := fs[5].Elems(keysConfigType.Field(5).Type)
	require.True(t, ok)
	require.Len(t, items, 2)

	byName, ok := fs[6].Entries(keysConfigType.Field(6).Type)
	require.True(t, ok)
	require.Contains(t, byName, "a")
}
