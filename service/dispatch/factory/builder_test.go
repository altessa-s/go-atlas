// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/service/dispatch/factory"
)

type mockSink struct{}

func (m *mockSink) StoreBatch(ctx context.Context, batch []string) error {
	return nil
}

func TestNew(t *testing.T) {
	t.Parallel()

	cfg := &config.Dispatch{
		BatchSize:     100,
		FlushInterval: 1 * time.Second,
		Workers:       4,
	}

	builder := factory.New[string](cfg)
	require.NotNil(t, builder)
}

func TestEngineBuilder_Build_RequiresSink(t *testing.T) {
	t.Parallel()

	cfg := &config.Dispatch{
		BatchSize:     100,
		FlushInterval: 1 * time.Second,
		Workers:       4,
	}

	builder := factory.New[string](cfg)
	_, err := builder.Build()
	require.Error(t, err)
	require.Contains(t, err.Error(), "sink")
}

func TestEngineBuilder_Build_Success(t *testing.T) {
	t.Parallel()

	cfg := &config.Dispatch{
		BatchSize:     100,
		FlushInterval: 1 * time.Second,
		Workers:       4,
	}

	sink := &mockSink{}
	builder := factory.New[string](cfg).
		WithSink(sink)

	engine, err := builder.Build()
	require.NoError(t, err)
	require.NotNil(t, engine)

	// Cleanup
	require.NoError(t, engine.Shutdown(context.Background()))
}

func TestEngineBuilder_Build_WithWAL(t *testing.T) {
	t.Parallel()

	cfg := &config.Dispatch{
		BatchSize:     100,
		FlushInterval: 1 * time.Second,
		Workers:       4,
		WAL: &config.WAL{
			Enabled: true,
			Dir:     t.TempDir(),
		},
	}

	sink := &mockSink{}
	codec := &mockCodec{}

	builder := factory.New[string](cfg).
		WithSink(sink).
		WithCodec(codec)

	engine, err := builder.Build()
	require.NoError(t, err)
	require.NotNil(t, engine)

	// Cleanup
	require.NoError(t, engine.Shutdown(context.Background()))
}

func TestEngineBuilder_Build_WALRequiresCodec(t *testing.T) {
	t.Parallel()

	cfg := &config.Dispatch{
		BatchSize:     100,
		FlushInterval: 1 * time.Second,
		Workers:       4,
		WAL: &config.WAL{
			Enabled: true,
			Dir:     t.TempDir(),
		},
	}

	sink := &mockSink{}

	builder := factory.New[string](cfg).
		WithSink(sink)
	// No codec provided

	_, err := builder.Build()
	require.Error(t, err)
	require.Contains(t, err.Error(), "codec")
}

type mockCodec struct{}

func (m *mockCodec) Encode(item string) ([]byte, error) {
	return []byte(item), nil
}

func (m *mockCodec) Decode(b []byte) (string, error) {
	return string(b), nil
}
