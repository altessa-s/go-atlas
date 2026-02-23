// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package yaml3 provides a YAML backend for the configuration parser.
// Uses gopkg.in/yaml.v3 for parsing.
//
// Example:
//
//	p := parser.New(&yaml3.Backend{}, parser.WithPath("config.yaml"))
//	p.Load(cfg)
package yaml3
