// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package yaml3_test

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader/backend"
	"github.com/altessa-s/go-atlas/config/loader/backend/yaml3"
)

type keysItem struct {
	Weight int `yaml:"weight"`
}

type keysBase struct {
	Port int `yaml:"port"`
}

type keysInline struct {
	Host string `yaml:"host"`
}

type keysConfig struct {
	keysBase                       // not inline: bound to "keysbase"
	keysInline `yaml:",inline"`    // flattened
	Name       string              `yaml:"name"`
	Skipped    string              `yaml:"-"`
	Untagged   string              // binds to "untagged"
	ByName     map[string]keysItem `yaml:"byName"`
	ByID       map[int]keysItem    `yaml:"byID"`
	ByFlag     map[bool]keysItem   `yaml:"byFlag"`
	Items      []keysItem          `yaml:"items"`
	PItems     []*keysItem         `yaml:"pItems"`
}

var keysConfigType = reflect.TypeFor[keysConfig]()

func decodeKeys(t *testing.T, doc string) backend.KeyNode {
	t.Helper()
	root, err := (&yaml3.Backend{}).DecodeKeys(strings.NewReader(doc))
	require.NoError(t, err)
	return root
}

func fields(t *testing.T, n backend.KeyNode, typ reflect.Type) map[int]backend.KeyNode {
	t.Helper()
	fs, ok := n.Fields(typ)
	require.True(t, ok)
	return fs
}

func entries(t *testing.T, n backend.KeyNode, typ reflect.Type) map[any]backend.KeyNode {
	t.Helper()
	es, ok := n.Entries(typ)
	require.True(t, ok)
	return es
}

func fieldIndex(t *testing.T, typ reflect.Type, name string) int {
	t.Helper()
	sf, ok := typ.FieldByName(name)
	require.True(t, ok)
	return sf.Index[0]
}

// Struct fields bind like yaml.v3: tag names, lowercased Go names, only
// ",inline" flattened, an embedded struct without it keyed by its lowercased
// type name, and "-" excluded.
func TestBackend_DecodeKeys_StructFields(t *testing.T) {
	t.Parallel()

	root := decodeKeys(t, "keysbase: {port: 0}\nhost: h\nname: \"\"\nSkipped: x\nskipped: x\nuntagged: u\nport: 1\n")
	fs := fields(t, root, keysConfigType)

	base := fields(t, fs[fieldIndex(t, keysConfigType, "keysBase")], reflect.TypeFor[keysBase]())
	require.Contains(t, base, 0, "embedded struct without ,inline is a keyed field")
	require.Len(t, base, 1)

	inline := fields(t, fs[fieldIndex(t, keysConfigType, "keysInline")], reflect.TypeFor[keysInline]())
	require.Contains(t, inline, 0, "inline struct fields bind from the parent mapping")

	require.Contains(t, fs, fieldIndex(t, keysConfigType, "Name"))
	require.Contains(t, fs, fieldIndex(t, keysConfigType, "Untagged"))
	require.NotContains(t, fs, fieldIndex(t, keysConfigType, "Skipped"))
}

// Map keys are decoded into the destination key type.
func TestBackend_DecodeKeys_TypedMapKeys(t *testing.T) {
	t.Parallel()

	root := decodeKeys(t, `
names: [&k weight]
byName:
  0x10: {}
  *k : {}
  !!binary bmFtZQ== : {}
  ~ : {}
byID:
  0x10: {}
byFlag:
  yes: {}
  off: {}
`)
	fs := fields(t, root, keysConfigType)

	byName := entries(t, fs[fieldIndex(t, keysConfigType, "ByName")], reflect.TypeFor[map[string]keysItem]())
	require.Contains(t, byName, "0x10", "a string key keeps its source spelling")
	require.Contains(t, byName, "weight", "an alias key resolves to its anchor's value")
	require.Contains(t, byName, "name", "a !!binary key is decoded")
	require.Len(t, byName, 3, "a null key is skipped")

	byID := entries(t, fs[fieldIndex(t, keysConfigType, "ByID")], reflect.TypeFor[map[int]keysItem]())
	require.Contains(t, byID, 16)

	byFlag := entries(t, fs[fieldIndex(t, keysConfigType, "ByFlag")], reflect.TypeFor[map[bool]keysItem]())
	require.Contains(t, byFlag, true)
	require.Contains(t, byFlag, false)
}

// "<<" merges follow yaml.v3: the mapping's own keys win — except a key whose
// resolved value differs from the decoded one, which the merge overrides.
func TestBackend_DecodeKeys_MergePrecedence(t *testing.T) {
	t.Parallel()

	root := decodeKeys(t, `
byName:
  <<: {a: {weight: 0}, 0x10: {weight: 0}}
  a: {}
  0x10: {}
`)
	fs := fields(t, root, keysConfigType)
	byName := entries(t, fs[fieldIndex(t, keysConfigType, "ByName")], reflect.TypeFor[map[string]keysItem]())

	itemType := reflect.TypeFor[keysItem]()
	require.Empty(t, fields(t, byName["a"], itemType), "the mapping's own entry wins")
	require.Contains(t, fields(t, byName["0x10"], itemType), 0, "yaml.v3 excludes merged keys by resolved value 16, so the merge wins")
}

// Like yaml.v3, a null sequence element is dropped unless the element type
// holds nulls.
func TestBackend_DecodeKeys_NullElements(t *testing.T) {
	t.Parallel()

	root := decodeKeys(t, "items: [~, {weight: 0}]\npItems: [~, {weight: 0}]\n")
	fs := fields(t, root, keysConfigType)

	items, ok := fs[fieldIndex(t, keysConfigType, "Items")].Elems(reflect.TypeFor[[]keysItem]())
	require.True(t, ok)
	require.Len(t, items, 1)
	require.False(t, items[0].IsNull())

	pItems, ok := fs[fieldIndex(t, keysConfigType, "PItems")].Elems(reflect.TypeFor[[]*keysItem]())
	require.True(t, ok)
	require.Len(t, pItems, 2)
	require.True(t, pItems[0].IsNull())
}

// A value containing itself through an alias stops binding instead of
// recursing forever.
func TestBackend_DecodeKeys_AliasCycle(t *testing.T) {
	t.Parallel()

	type nested []nested
	root := decodeKeys(t, "x: &x [*x]\n")
	es, ok := root.Entries(reflect.TypeFor[map[string]nested]())
	require.True(t, ok)

	n := es["x"]
	for range 3 {
		elems, ok := n.Elems(reflect.TypeFor[nested]())
		if !ok {
			return
		}
		require.Len(t, elems, 1)
		n = elems[0]
	}
	t.Fatal("alias cycle was followed more than once")
}

// An empty document is a null value.
func TestBackend_DecodeKeys_Empty(t *testing.T) {
	t.Parallel()

	require.True(t, decodeKeys(t, "").IsNull())
	require.False(t, decodeKeys(t, "a: 1\n").IsNull())
}

// Nodes of one document may be bound concurrently.
func TestBackend_DecodeKeys_Concurrent(t *testing.T) {
	t.Parallel()

	root := decodeKeys(t, "base: &b {weight: 0}\nbyName:\n  a: *b\nitems:\n  - <<: *b\n")
	fs := fields(t, root, keysConfigType)
	byName := fs[fieldIndex(t, keysConfigType, "ByName")]
	items := fs[fieldIndex(t, keysConfigType, "Items")]

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, _ = root.Fields(keysConfigType)
			_, _ = byName.Entries(reflect.TypeFor[map[string]keysItem]())
			_, _ = items.Elems(reflect.TypeFor[[]keysItem]())
		})
	}
	wg.Wait()
}
