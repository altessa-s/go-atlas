// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the lockbox section of the secrets configuration to a
// Yandex Cloud Lockbox provider for factory/ext/kryptos.
//
// The secrets factory builds only the in-memory provider; register this one
// for the lockbox provider type:
//
//	manager, err := secretsfactory.New(&cfg.Secrets).
//		UseProvider(secretsconfig.ProviderLockbox, lockboxfactory.Builder()).
//		Build(ctx)
package factory
