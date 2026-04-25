// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package msg

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewMessageWithMeta_CreatedTimeCanBeOverridden(t *testing.T) {
	override := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC).Format(MessageCreatedTimeFormat)

	m := NewMessageWithMeta("t", []byte("x"), Meta{
		{Key: MetaKeyMessageCreatedTime, Value: override},
	})

	got, ok := m.Metadata.Value(MetaKeyMessageCreatedTime)
	require.True(t, ok, "missing %s", MetaKeyMessageCreatedTime)
	require.Equal(t, override, got)
}

func TestNewMessageWithMeta_DeduplicatesByKeyPreservingFirst(t *testing.T) {
	m := NewMessageWithMeta("t", []byte("x"), Meta{
		{Key: "k", Value: "1"},
		{Key: "k", Value: "2"},
	})

	got, ok := m.Metadata.Value("k")
	require.True(t, ok, "missing key k")
	require.Equal(t, "1", got)
}

func TestNewMessageWithMeta_DataIsCopied(t *testing.T) {
	in := []byte("hello")
	m := NewMessageWithMeta("t", in, nil)
	in[0] = 'H'
	require.Equal(t, "hello", string(m.Data))
}
