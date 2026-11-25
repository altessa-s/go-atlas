// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/retryqueue"
	"github.com/altessa-s/go-atlas/reliability/retryqueue/storages/memory"
	"github.com/altessa-s/go-atlas/reliability/retryqueue/storages/storagetest"
)

func TestPayloadSnapshotPreservesCyclesAndHandles(t *testing.T) {
	t.Parallel()

	type node struct {
		Next   *node
		Values map[string]any
		Handle chan struct{}
	}
	payload := &node{Handle: make(chan struct{})}
	payload.Next = payload
	payload.Values = map[string]any{"bytes": []byte{1}}
	payload.Values["cycle"] = payload.Values
	store := memory.New[*node]()
	item := retryqueue.Item[*node]{Key: "cyclic", Payload: payload}
	marked, err := store.Mark(t.Context(), item)
	require.NoError(t, err)
	require.Same(t, marked.Payload, marked.Payload.Next)
	require.NotSame(t, payload, marked.Payload)
	require.Equal(t, payload.Handle, marked.Payload.Handle)
	marked.Payload.Values["cycle"].(map[string]any)["bytes"].([]byte)[0] = 2
	require.Equal(t, byte(2), marked.Payload.Values["bytes"].([]byte)[0])
	require.Equal(t, byte(1), payload.Values["bytes"].([]byte)[0])

	due, err := store.ListDue(t.Context(), storagetest.T0, 1)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, []byte{1}, due[0].Payload.Values["bytes"])
}

func TestPayloadSnapshotUsesCustomClone(t *testing.T) {
	t.Parallel()

	type payload struct{ private []byte }
	store := memory.NewWithClone(func(p payload) payload {
		return payload{private: append([]byte(nil), p.private...)}
	})
	item := retryqueue.Item[payload]{Key: "opaque", Payload: payload{private: []byte{1}}}
	marked, err := store.Mark(t.Context(), item)
	require.NoError(t, err)
	item.Payload.private[0] = 2
	marked.Payload.private[0] = 3

	due, err := store.ListDue(t.Context(), storagetest.T0, 1)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, []byte{1}, due[0].Payload.private)
}

func TestPayloadSnapshotPreservesMapKeyReferences(t *testing.T) {
	t.Parallel()
	type key struct{ id int }
	type indexKey struct{ identity [1]*key }
	type payload struct {
		Selected *key
		Index    map[any]int
		Other    *key
	}
	selected := &key{id: 1}
	item := retryqueue.Item[payload]{Key: "selected", Payload: payload{
		Selected: selected,
		Index:    map[any]int{indexKey{[1]*key{selected}}: 42},
		Other:    &key{id: 2},
	}}
	store := memory.New[payload]()
	marked, err := store.Mark(t.Context(), item)
	require.NoError(t, err)
	items, err := store.ListByState(t.Context(), retryqueue.StatePending, 1)
	require.NoError(t, err)
	require.Len(t, items, 1)
	for _, got := range []payload{marked.Payload, items[0].Payload} {
		require.Same(t, selected, got.Selected)
		require.Equal(t, 42, got.Index[indexKey{[1]*key{got.Selected}}])
		require.NotSame(t, item.Payload.Other, got.Other)
	}
}

func TestLimitedListsExposeOnlyClonedItems(t *testing.T) {
	t.Parallel()
	store := memory.New[[]byte]()
	for i, key := range []string{"a", "b", "c"} {
		_, err := store.Mark(t.Context(), retryqueue.Item[[]byte]{Key: key, Payload: []byte{byte(i + 1)}})
		require.NoError(t, err)
	}
	now := time.Now()
	due, err := store.ListDue(t.Context(), now, 1)
	require.NoError(t, err)
	pending, err := store.ListByState(t.Context(), retryqueue.StatePending, 1)
	require.NoError(t, err)
	for _, list := range [][]retryqueue.Item[[]byte]{due, pending} {
		require.Len(t, list, 1)
		require.Equal(t, len(list), cap(list))
		list[0].Payload[0] = 9
	}
	got, err := store.ListDue(t.Context(), now, 3)
	require.NoError(t, err)
	require.Len(t, got, 3)
	for i := range got {
		require.Equal(t, []byte{byte(i + 1)}, got[i].Payload)
	}
}
