// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"iter"
	"log/slog"
)

// SPIVersion declares the contract version that a plugin provider
// implements. Plugins export it as a companion symbol alongside the
// provider symbol using the naming convention "<Symbol>SPIVersion":
//
//	var AuthProvider         = &myAuthProvider{}
//	var AuthProviderSPIVersion = plugins.SPIVersion{
//	    Contract: "AuthProvider",
//	    Major:    2,
//	    Minor:    1,
//	}
//
// [Manager] does not interpret SPIVersion directly; version negotiation
// is performed by the free function [NegotiateAll], which wraps
// [Manager.LookupAll] with constraint filtering.
type SPIVersion struct {
	// Contract is the symbol name this version applies to
	// (e.g. "AuthProvider"). Informational — not used for matching.
	Contract string

	// Major is incremented on breaking changes. A major mismatch
	// between host and plugin is always incompatible.
	Major int

	// Minor is incremented on backwards-compatible additions.
	// The host's [SPIConstraint.MinMinor] sets the floor.
	Minor int
}

// SPIConstraint declares the host's version requirements for an SPI
// contract. Use it with [NegotiateAll] to filter out incompatible
// providers.
type SPIConstraint struct {
	// Major is the exact major version the host requires.
	Major int

	// MinMinor is the minimum minor version the host accepts.
	MinMinor int
}

// Matches reports whether v satisfies the constraint. Rules: major must
// match exactly; minor must be >= MinMinor.
func (c SPIConstraint) Matches(v SPIVersion) bool {
	return v.Major == c.Major && v.Minor >= c.MinMinor
}

// NegotiateAll wraps [Manager.LookupAll] with SPI version negotiation.
// For each ready plugin that exports symbol, it also looks up
// "<symbol>SPIVersion". If the version symbol is present and does not
// satisfy the constraint, the provider is skipped and a warning is
// logged. If the version symbol is absent, the provider is included
// unchanged — this preserves backwards compatibility with unversioned
// plugins.
//
// Example:
//
//	constraint := plugins.SPIConstraint{Major: 2, MinMinor: 0}
//	for p, sym := range plugins.NegotiateAll(mgr, "AuthProvider", constraint, logger) {
//	    provider, ok := sym.(auth.Provider)
//	    if !ok { continue }
//	    // provider is guaranteed version-compatible
//	}
func NegotiateAll(mgr *Manager, symbol string, constraint SPIConstraint, logger *slog.Logger) iter.Seq2[*Plugin, any] {
	versionSymbol := symbol + "SPIVersion"
	return func(yield func(*Plugin, any) bool) {
		for p, sym := range mgr.LookupAll(symbol) {
			verSym, ok := p.Lookup(versionSymbol)
			if !ok {
				// No version symbol → unversioned plugin → include.
				if !yield(p, sym) {
					return
				}
				continue
			}

			ver, valid := unwrapSPIVersion(verSym)
			if !valid {
				logger.Warn("plugin exports malformed SPI version symbol; skipping provider",
					slog.String("plugin", p.Name()),
					slog.String("symbol", versionSymbol),
				)
				continue
			}
			if !constraint.Matches(ver) {
				logger.Warn("plugin SPI version does not satisfy host constraint; skipping provider",
					slog.String("plugin", p.Name()),
					slog.String("contract", symbol),
					slog.Int("plugin_major", ver.Major),
					slog.Int("plugin_minor", ver.Minor),
					slog.Int("host_major", constraint.Major),
					slog.Int("host_min_minor", constraint.MinMinor),
				)
				continue
			}

			if !yield(p, sym) {
				return
			}
		}
	}
}

// unwrapSPIVersion handles both *SPIVersion and **SPIVersion forms
// from [Plugin.Lookup], mirroring the pattern used in
// [resolveDescriptor].
func unwrapSPIVersion(sym any) (SPIVersion, bool) {
	switch v := sym.(type) {
	case *SPIVersion:
		if v == nil {
			return SPIVersion{}, false
		}
		return *v, true
	case **SPIVersion:
		if v == nil || *v == nil {
			return SPIVersion{}, false
		}
		return **v, true
	default:
		return SPIVersion{}, false
	}
}
