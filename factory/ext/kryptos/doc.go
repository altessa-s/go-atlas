// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory builds a github.com/altessa-s/go-kryptos manager.Manager
// from the go-atlas secrets configuration.
//
// go-kryptos starts no background loop: with [ManagerBuilder.UseScheduler]
// the builder registers the manager's RunUpdateCycle as the [UpdateTaskID]
// task, and with [ManagerBuilder.UseHealthCoordinator] it registers the
// manager's health as the [HealthServiceName] service.
//
// The builder builds only the in-memory provider. The remote providers (Vault,
// GCP Secret Manager, Yandex Cloud Lockbox) register their [ProviderBuilder]
// per provider type with [ManagerBuilder.UseProvider], from
// factory/ext/kryptos/<provider>; a configured provider without
// one fails at Build naming the package to add.
//
// [ManagerBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [ManagerBuilder.Build] time.
//
//	manager, err := factory.New[any](&cfg.Secrets).
//	    UseLogger(logger).
//	    UseScheduler(sched).
//	    UseProvider(secretsconfig.ProviderVault, vaultfactory.Builder[any](vaultClient)).
//	    Build(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer manager.Close(context.WithoutCancel(ctx))
//
// [New] instantiated with string builds the string-typed Manager the config
// loader expands $__secret{namespace:key} placeholders with:
//
//	secretsManager, err := factory.New[string](&cfg.Secrets).Build(ctx)
//	if err != nil {
//	    return err
//	}
//	defer secretsManager.Close(context.WithoutCancel(ctx))
//	l := loader.New(backend, loader.WithSecretResolver(manager.NewStringResolver(secretsManager)))
//
// Closing the manager releases its health registration and stops its local
// updates. The shared scheduler task stays registered for other managers.
package factory
