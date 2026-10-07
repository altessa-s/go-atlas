// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package providertest is the contract suite for [providers.Provider]
// implementations. Every bundled lock backend runs it, and a custom one should
// too: [dlock] relies on exclusive acquisition, monotonic fencing tokens,
// background renewal and release on context end or Close, which a real store
// must get right under contention.
//
// [Run] executes every contract as parallel subtests, each on a fresh
// [Backend] — an isolated namespace from which several providers contend like
// replicas of one service. Contracts that age a lease out from under its
// holder need [Backend.Expire] and are skipped without it. Each contract is
// also exported on its own.
//
// # Usage
//
//	func TestProviderContract(t *testing.T) {
//		t.Parallel()
//		providertest.Run(t, func(tb testing.TB) providertest.Backend {
//			ns := newNamespace(tb)
//			return providertest.Backend{NewProvider: ns.provider, Expire: ns.expire}
//		})
//	}
package providertest
