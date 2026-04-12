// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsbase_test

import (
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/internal/natsbase"
)

// mockKV is a minimal mock of jetstream.KeyValue for testing NewBase.
type mockKV struct {
	jetstream.KeyValue
}

func TestNewBase(t *testing.T) {
	kv := &mockKV{}
	base := natsbase.NewBase(kv)

	require.Equal(t, kv, base.KV(), "KV() should return the provided KeyValue")
}

func TestBase_KV_ReturnsProvided(t *testing.T) {
	kv := &mockKV{}
	base := natsbase.NewBase(kv)

	require.NotNil(t, base.KV())
}
