// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package opa provides Open Policy Agent (OPA) integration for authorization.
//
// It supports policy evaluation from various sources (filesystem, HTTP bundles)
// with hot-reloading capabilities. The package follows a modular design with
// pluggable policy sources and event-driven architecture for policy updates.
//
// # Architecture
//
// The package is built around three main components:
//
//   - [Manager]: Orchestrates policy loading, caching, and evaluation
//   - [PolicySource]: Interface for fetching policies from various backends
//   - [Evaluator]: Thread-safe policy evaluation interface
//
// # Quick Start
//
// Basic usage with filesystem source:
//
//	source, err := filesystem.New("/path/to/policies")
//	if err != nil {
//	    return err
//	}
//
//	manager, err := opa.NewManager(ctx, source, "data.authz.allow")
//	if err != nil {
//	    return err
//	}
//	defer manager.Close()
//
//	// Evaluate a policy
//	result, err := manager.Evaluator().Evaluate(ctx, map[string]any{
//	    "user":   "alice",
//	    "action": "read",
//	})
//	if result.Allow {
//	    // Access granted
//	}
//
// # Hot Reloading
//
// Enable automatic policy reloading when files change:
//
//	if err := manager.StartWatching(ctx); err != nil {
//	    return err
//	}
//
//	// Subscribe to policy events
//	watch, _ := manager.Watch(ctx, opa.DefaultWatchOptions())
//	defer watch.Stop()
//
//	for event := range watch.Events {
//	    switch event.Type {
//	    case opa.EventTypePolicyUpdated:
//	        log.Printf("Policies updated: %s", event.Revision)
//	    case opa.EventTypePolicyError:
//	        log.Printf("Policy error: %v", event.Error)
//	    }
//	}
//
// # Factory Usage
//
// For configuration-driven setup, use the factory package:
//
//	manager, err := factory.New(cfg.OPA).
//	    UseLogger(logger).
//	    UseScheduler(scheduler).
//	    Build(ctx)
//
// # Thread Safety
//
// All public methods on [Manager] and [Evaluator] are safe for concurrent use.
// Policy updates are applied atomically, ensuring consistent evaluation results.
package opa
