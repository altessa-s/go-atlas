// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package transform provides string-transformation plugins that wrap input
// expressions in standard library calls during code generation.
//
// Each plugin implements [plugin.TransformPlugin] and is composed into a chain
// by [builtin.BuildStringTransformChain]. Transforms are applied in tag order,
// innermost first:
//
//	optval:"lower,trimspaces"  =>  strings.TrimSpace(strings.ToLower(v))
//
// Built-in transforms:
//
//   - [LowerTransformPlugin] (lower) -- strings.ToLower
//   - [UpperTransformPlugin] (upper) -- strings.ToUpper
//   - [TrimSpacesTransformPlugin] (trimspaces) -- strings.TrimSpace; applied by
//     default to all string fields, disabled with optval:"notrim"
//   - [TrimPrefixTransformPlugin] (trimprefix=X) -- strings.TrimPrefix with literal X
//   - [TrimSuffixTransformPlugin] (trimsuffix=X) -- strings.TrimSuffix with literal X
//
// # Tag Usage
//
//	type Options struct {
//	    Name   string `optval:"lower,trimspaces"`
//	    Path   string `optval:"trimprefix=/"`
//	    Domain string `optval:"trimsuffix=."`
//	}
package transform
