// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=parserConfig --output=parser_options_gen.go --option-type=ParserOption

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// DefaultParserCacheSize is the maximum number of parsed AST nodes to cache.
const DefaultParserCacheSize = 1000

// parserConfig holds [Parser] configuration. All fields are populated
// either by generated [ParserOption] setters in parser_options_gen.go or
// by the hand-written setters in this file (those tagged optgen:"manual").
type parserConfig struct {
	cacheSize                 int                       `opt:"ParserCacheSize" optgen:"default=DefaultParserCacheSize" optval:"positive"`
	maxExpressionLength       int                       `opt:"MaxExpressionLength" optgen:"default=DefaultMaxExpressionLength" optval:"positive"`
	noCache                   bool                      `opt:"ParserNoCache"`
	collector                 metrics.Collector         `opt:"ParserCollector"`
	customFunctions           map[string]CustomFunction //nolint:unused // populated via WithCustomFunctions generated option
	allowedFunctions          map[string]struct{}       `optgen:"manual"`
	skipGlobalCustomFunctions bool                      `optgen:"manual"`
}

// WithAllowedFunctions restricts which CEL functions the parser
// accepts. The whitelist covers built-in named functions (contains,
// startsWith, endsWith, matches, size, timestamp) and any handlers
// registered via [WithCustomFunctions]. Operators (==, !=, <, &&, ||,
// !, in) and the has() macro are baseline grammar and are always
// allowed.
//
// When this option is not set, every function reachable from the
// parser is accepted (the historical default). A name registered as a
// custom function but absent from the whitelist is rejected at parse
// time with [ErrFunctionNotAllowed], not at [NewParser] — that keeps
// the "global registry + per-parser narrowing" pattern available.
//
// Example — only allow safe substring queries plus a custom shortcut:
//
//	parser, _ := filter.NewParser(
//	    filter.WithAllowedFunctions("contains", "startsWith", "createdAfter"),
//	    filter.WithCustomFunctions(map[string]filter.CustomFunction{
//	        "createdAfter": filter.CompareField("createdAt", filter.OpGT),
//	    }),
//	)
func WithAllowedFunctions(names ...string) ParserOption {
	return func(c *parserConfig) {
		c.allowedFunctions = make(map[string]struct{}, len(names))
		for _, n := range names {
			c.allowedFunctions[n] = struct{}{}
		}
	}
}

// WithoutGlobalCustomFunctions excludes the package-level registry
// (populated via [RegisterFunctions]) from this parser's effective
// function set. The parser then only sees functions passed via
// [WithCustomFunctions]. Useful in tests and for isolated parsers
// that should not pick up application-wide registrations.
func WithoutGlobalCustomFunctions() ParserOption {
	return func(c *parserConfig) {
		c.skipGlobalCustomFunctions = true
	}
}
