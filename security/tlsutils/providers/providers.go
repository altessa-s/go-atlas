// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsproviders

import (
	"context"
	"iter"
	"sync"
)

// ProviderType identifies the type of TLS certificate provider.
// Use the predefined constants for supported provider types.
type ProviderType string

const (
	// ProviderTypeVault is the Vault-based certificate provider.
	ProviderTypeVault ProviderType = "vault"
	// ProviderTypeFile is the file-based certificate provider.
	ProviderTypeFile ProviderType = "file"
	// ProviderTypeLetsEncrypt is the Let's Encrypt certificate provider.
	ProviderTypeLetsEncrypt ProviderType = "letsencrypt"
)

// String returns the string representation of the provider type.
// Implements the fmt.Stringer interface.
//
// Example:
//
//	fmt.Println(ProviderTypeVault.String()) // "vault"
func (t ProviderType) String() string {
	return string(t)
}

// IsValid returns true if the provider type is a supported provider.
// Used for validating provider type configurations.
//
// Example:
//
//	if !providerType.IsValid() {
//		return errors.New("unsupported provider type")
//	}
func (t ProviderType) IsValid() bool {
	switch t {
	case ProviderTypeVault, ProviderTypeFile, ProviderTypeLetsEncrypt:
		return true
	}
	return false
}

// AvailableProviders is the list of all available provider types.
// Use this to enumerate or display supported provider types.
var AvailableProviders = []ProviderType{
	ProviderTypeVault,
	ProviderTypeFile,
	ProviderTypeLetsEncrypt,
}

// Providers is a collection of TLS providers.
// It provides thread-safe registration and lookup of providers by type.
type Providers struct {
	mu        sync.RWMutex
	providers map[ProviderType]Provider
}

// Register registers a TLS provider with the provider collection.
// The provider's type is used as the registry key.
//
// Example:
//
//	providers.Register(fileProvider)
func (p *Providers) Register(provider Provider) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.providers == nil {
		p.providers = make(map[ProviderType]Provider)
	}
	p.providers[provider.Type()] = provider
}

// Get returns a provider by type.
// Returns false if the provider type is not registered.
//
// Example:
//
//	prov, ok := providers.Get(ProviderTypeFile)
//	if ok {
//		config, _ := prov.TLSConfig()
//	}
func (p *Providers) Get(t ProviderType) (Provider, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	provider, ok := p.providers[t]
	return provider, ok
}

// Close closes all registered providers with the given context.
// The context controls the graceful shutdown timeout for each provider.
// The errCallback is invoked for each provider that returns an error.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	providers.Close(ctx, func(p Provider, err error) {
//		log.Printf("error closing %s: %v", p.Type(), err)
//	})
func (p *Providers) Close(ctx context.Context, errCallback func(p Provider, err error)) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, provider := range p.providers {
		if err := provider.Close(ctx); err != nil && errCallback != nil {
			errCallback(provider, err)
		}
	}
}

// List returns an iterator over all registered providers.
// The order of providers is not guaranteed.
//
// Example:
//
//	for prov := range providers.List() {
//		fmt.Println(prov.Type())
//	}
func (p *Providers) List() iter.Seq[Provider] {
	return func(yield func(Provider) bool) {
		p.mu.RLock()
		defer p.mu.RUnlock()

		for _, prov := range p.providers {
			if !yield(prov) {
				return
			}
		}
	}
}
