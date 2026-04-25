// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package toml provides a TOML backend for the configuration parser.
// Uses github.com/BurntSushi/toml for parsing.
//
// Example:
//
//	p := parser.New(&toml.Backend{}, parser.WithPath("config.toml"))
//	p.Load(cfg)
package toml
