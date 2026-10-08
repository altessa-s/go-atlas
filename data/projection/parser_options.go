// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package projection

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=parserOptions --output=parser_options_gen.go --option-type=ParserOption

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// DefaultParserCacheSize is the default maximum number of parsed Spec
// values held in the parser's LRU cache.
const DefaultParserCacheSize = 1000

// DefaultMaxExpressionLength is the default maximum allowed length (in
// bytes) of a raw fields expression. It is a safety stop against
// pathologically long input, well above any sensible field list.
const DefaultMaxExpressionLength = 4096

// DefaultMaxPaths is the default maximum number of paths accepted in a
// single expression, counted before deduplication.
const DefaultMaxPaths = 64

// DefaultMaxFieldPathDepth is the default maximum number of dot-separated
// segments in a single path (e.g. "a.b.c" has depth 3).
const DefaultMaxFieldPathDepth = 8

// DefaultMaxFieldNameLength is the default maximum allowed length (in
// bytes) of a single path including its dots.
const DefaultMaxFieldNameLength = 128

// parserOptions holds [Parser] configuration. All fields are populated by
// the generated [ParserOption] setters in parser_options_gen.go.
type parserOptions struct {
	cacheSize           int               `opt:"ParserCacheSize" optgen:"default=DefaultParserCacheSize" optval:"positive"`
	noCache             bool              `opt:"ParserNoCache"`
	maxExpressionLength int               `opt:"MaxExpressionLength" optgen:"default=DefaultMaxExpressionLength" optval:"positive"`
	maxPaths            int               `opt:"MaxPaths" optgen:"default=DefaultMaxPaths" optval:"positive"`
	maxFieldPathDepth   int               `opt:"MaxFieldPathDepth" optgen:"default=DefaultMaxFieldPathDepth" optval:"positive"`
	maxFieldNameLength  int               `opt:"MaxFieldNameLength" optgen:"default=DefaultMaxFieldNameLength" optval:"positive"`
	collector           metrics.Collector `opt:"ParserCollector" optgen:"notnil"`
}
