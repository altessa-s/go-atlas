// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package main provides optgen, a code generator for the functional option pattern.
//
// optgen scans Go structs for fields annotated with opt/optgen/optval/optcheck tags and
// generates type-safe WithXxx option functions, default constructors, and validation code.
// It supports a plugin system for extensible field handling, value transforms, and checks.
//
// Typical usage via go:generate:
//
//	//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=Options
//
// See the [commands] package for CLI flags and the [plugin] package for extension points.
package main
