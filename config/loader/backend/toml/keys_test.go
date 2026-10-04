// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package toml_test

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	burntsushi "github.com/BurntSushi/toml"
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

type collisionConfig struct {
	KeysFlat
	Embedded_0 int `toml:"e"` //nolint:revive,staticcheck // collides with a synthetic shadow name
}

// An embedded struct and a field named like a synthetic shadow field do not
// collide.
func TestBackend_DecodeKeys_ShadowNames(t *testing.T) {
	t.Parallel()

	root, err := (&toml.Backend{}).DecodeKeys(strings.NewReader("port = 0\ne = 0\n"))
	require.NoError(t, err)
	fs, ok := root.Fields(reflect.TypeFor[collisionConfig]())
	require.True(t, ok)

	flat, ok := fs[0].Fields(reflect.TypeFor[KeysFlat]())
	require.True(t, ok)
	require.Contains(t, flat, 0)
	require.Contains(t, fs, 1)
}

// A tag is kept whole: "-,omitempty" is the literal key "-".
func TestBackend_DecodeKeys_DashTag(t *testing.T) {
	t.Parallel()

	type dashConfig struct {
		Dash int `toml:"-,omitempty"`
	}
	root, err := (&toml.Backend{}).DecodeKeys(strings.NewReader("\"-\" = 0\n"))
	require.NoError(t, err)
	fs, ok := root.Fields(reflect.TypeFor[dashConfig]())
	require.True(t, ok)
	require.Contains(t, fs, 0, `"-,omitempty" binds the key "-"`)
}

// Nodes of one document may be bound concurrently.
func TestBackend_DecodeKeys_Concurrent(t *testing.T) {
	t.Parallel()

	root, err := (&toml.Backend{}).DecodeKeys(strings.NewReader("port = 0\n[[items]]\nZ = 0\n[byName.a]\nport = 0\n"))
	require.NoError(t, err)
	fs, ok := root.Fields(keysConfigType)
	require.True(t, ok)

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, _ = root.Fields(keysConfigType)
			_, _ = fs[5].Elems(keysConfigType.Field(5).Type)
			_, _ = fs[6].Entries(keysConfigType.Field(6).Type)
		})
	}
	wg.Wait()
}

type (
	DiamondLeaf struct {
		X int
	}
	DiamondLeft struct {
		DiamondLeaf
	}
	DiamondRight struct {
		DiamondLeaf
	}
	diamondConfig struct {
		DiamondLeft
		DiamondRight
		Y int
	}
	ShallowInner struct {
		X int
	}
	shallowConfig struct {
		ShallowInner
		X int
	}
	TaggedLeft struct {
		X int `toml:"x"`
	}
	UntaggedRight struct {
		X int
	}
	taggedConfig struct {
		TaggedLeft
		UntaggedRight
	}
	Recursive struct {
		*Recursive
		Name string
	}
	MutualA struct {
		*MutualB
		A int
	}
	MutualB struct {
		*MutualA
		B int
	}
)

// leafSet reports whether the decoded fields hold a value at the struct index
// path, following flattened embedded structs.
func leafSet(t *testing.T, fs map[int]backend.KeyNode, typ reflect.Type, path ...int) bool {
	t.Helper()
	n, ok := fs[path[0]]
	if !ok || len(path) == 1 {
		return ok
	}
	ft := typ.Field(path[0]).Type
	for ft.Kind() == reflect.Pointer {
		ft = ft.Elem()
	}
	inner, ok := n.Fields(ft)
	require.True(t, ok)
	return leafSet(t, inner, ft, path[1:]...)
}

// Embedding resolution matches what BurntSushi/toml itself decodes: an
// ambiguous diamond binds nothing, a shallower field wins, a tagged field
// dominates an untagged one, and recursive embeddings terminate.
func TestBackend_DecodeKeys_EmbeddingMatchesDecoder(t *testing.T) {
	t.Parallel()

	keys := func(t *testing.T, doc string, typ reflect.Type) map[int]backend.KeyNode {
		t.Helper()
		root, err := (&toml.Backend{}).DecodeKeys(strings.NewReader(doc))
		require.NoError(t, err)
		fs, ok := root.Fields(typ)
		require.True(t, ok)
		return fs
	}

	t.Run("diamond", func(t *testing.T) {
		t.Parallel()
		doc := "X = 1\nY = 2\n"
		var real diamondConfig
		_, err := burntsushi.Decode(doc, &real)
		require.NoError(t, err)
		typ := reflect.TypeFor[diamondConfig]()
		fs := keys(t, doc, typ)
		require.Equal(t, real.DiamondLeft.X != 0, leafSet(t, fs, typ, 0, 0, 0))
		require.Equal(t, real.DiamondRight.X != 0, leafSet(t, fs, typ, 1, 0, 0))
		require.True(t, leafSet(t, fs, typ, 2))
	})
	t.Run("shallower wins", func(t *testing.T) {
		t.Parallel()
		doc := "X = 1\n"
		var real shallowConfig
		_, err := burntsushi.Decode(doc, &real)
		require.NoError(t, err)
		typ := reflect.TypeFor[shallowConfig]()
		fs := keys(t, doc, typ)
		require.Equal(t, real.ShallowInner.X != 0, leafSet(t, fs, typ, 0, 0))
		require.Equal(t, real.X != 0, leafSet(t, fs, typ, 1))
	})
	t.Run("tagged dominates", func(t *testing.T) {
		t.Parallel()
		doc := "x = 1\n"
		var real taggedConfig
		_, err := burntsushi.Decode(doc, &real)
		require.NoError(t, err)
		typ := reflect.TypeFor[taggedConfig]()
		fs := keys(t, doc, typ)
		require.Equal(t, real.TaggedLeft.X != 0, leafSet(t, fs, typ, 0, 0))
		require.Equal(t, real.UntaggedRight.X != 0, leafSet(t, fs, typ, 1, 0))
	})
	t.Run("recursive", func(t *testing.T) {
		t.Parallel()
		doc := "Name = \"a\"\n"
		var real Recursive
		_, err := burntsushi.Decode(doc, &real)
		require.NoError(t, err)
		typ := reflect.TypeFor[Recursive]()
		require.Equal(t, real.Name != "", leafSet(t, keys(t, doc, typ), typ, 1))
	})
	t.Run("mutual recursion", func(t *testing.T) {
		t.Parallel()
		doc := "A = 1\nB = 2\n"
		var real MutualA
		_, err := burntsushi.Decode(doc, &real)
		require.NoError(t, err)
		typ := reflect.TypeFor[MutualA]()
		fs := keys(t, doc, typ)
		require.Equal(t, real.A != 0, leafSet(t, fs, typ, 1))
		require.Equal(t, real.MutualB != nil && real.B != 0, leafSet(t, fs, typ, 0, 1))
	})
}
