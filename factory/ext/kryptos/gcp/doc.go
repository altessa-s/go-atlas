// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the gcp section of the secrets configuration to a
// GCP Secret Manager provider for factory/ext/kryptos.
//
// The secrets factory builds only the in-memory provider; register this one
// for the gcp provider type:
//
//	manager, err := secretsfactory.New[any](&cfg.Secrets).
//		UseProvider(secretsconfig.ProviderGCP, gcpfactory.Builder[any]()).
//		Build(ctx)
package factory
