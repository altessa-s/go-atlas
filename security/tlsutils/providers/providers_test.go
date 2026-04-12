// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsproviders

import (
	"context"
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockProvider struct {
	typ ProviderType
}

func (m *mockProvider) Type() ProviderType              { return m.typ }
func (m *mockProvider) TLSConfig() (*tls.Config, error) { return &tls.Config{}, nil } //nolint:gosec
func (m *mockProvider) Close(_ context.Context) error   { return nil }

func TestProviderType_String(t *testing.T) {
	require.Equal(t, "vault", ProviderTypeVault.String())
}

func TestProviderType_IsValid(t *testing.T) {
	tests := []struct {
		name string
		pt   ProviderType
		want bool
	}{
		{"vault", ProviderTypeVault, true},
		{"file", ProviderTypeFile, true},
		{"letsencrypt", ProviderTypeLetsEncrypt, true},
		{"s3", ProviderTypeS3, true},
		{"unknown", ProviderType("unknown"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.pt.IsValid())
		})
	}
}

func TestAvailableProviders(t *testing.T) {
	require.Len(t, AvailableProviders, 4)
}

func TestProviders_RegisterAndGet(t *testing.T) {
	p := &Providers{}
	mp := &mockProvider{typ: ProviderTypeFile}
	p.Register(mp)

	got, ok := p.Get(ProviderTypeFile)
	require.True(t, ok)
	require.Equal(t, ProviderTypeFile, got.Type())
}

func TestProviders_Get_NotFound(t *testing.T) {
	p := &Providers{}
	_, ok := p.Get(ProviderTypeVault)
	require.False(t, ok)
}

func TestProviders_Close(t *testing.T) {
	p := &Providers{}
	p.Register(&mockProvider{typ: ProviderTypeFile})
	p.Register(&mockProvider{typ: ProviderTypeVault})

	var errCount int
	p.Close(t.Context(), func(_ Provider, _ error) {
		errCount++
	})
	require.Equal(t, 0, errCount)
}

func TestProviders_List(t *testing.T) {
	p := &Providers{}
	p.Register(&mockProvider{typ: ProviderTypeFile})
	p.Register(&mockProvider{typ: ProviderTypeVault})

	count := 0
	for range p.List() {
		count++
	}
	require.Equal(t, 2, count)
}

func BenchmarkProviders_Get(b *testing.B) {
	p := &Providers{}
	p.Register(&mockProvider{typ: ProviderTypeFile})
	for b.Loop() {
		p.Get(ProviderTypeFile)
	}
}
