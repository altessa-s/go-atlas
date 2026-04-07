// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a configuration-driven builder for [plugins.Manager].
//
// # Usage
//
//	mgr, err := factory.NewManager(cfg).
//	    UseLogger(logger).
//	    Build(ctx)
package factory
