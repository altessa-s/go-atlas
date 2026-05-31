// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package orderby

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=parserOptions --output=parser_options_gen.go --option-type=ParserOption

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// DefaultParserCacheSize is the default maximum number of parsed Spec
// values held in the parser's LRU cache.
const DefaultParserCacheSize = 1000

// DefaultMaxExpressionLength is the default maximum allowed length (in
// bytes) for a raw order_by string. AIP-132 order_by strings are short by
// design — this default is well above any sensible production payload and
// is meant only as a safety stop against pathologically long input.
const DefaultMaxExpressionLength = 1024

// DefaultMaxKeys is the default maximum number of sort keys accepted in a
// single order_by string.
const DefaultMaxKeys = 32

// DefaultMaxFieldPathDepth is the default maximum number of dot-separated
// segments in a single field path (e.g. "a.b.c" has depth 3).
const DefaultMaxFieldPathDepth = 8

// DefaultMaxFieldNameLength is the default maximum allowed length (in
// bytes) of a single field path including its dots.
const DefaultMaxFieldNameLength = 128

// parserOptions holds [Parser] configuration. All fields are populated by
// the generated [ParserOption] setters in parser_options_gen.go.
type parserOptions struct {
	cacheSize              int               `opt:"ParserCacheSize" optgen:"default=DefaultParserCacheSize" optval:"positive"`
	noCache                bool              `opt:"ParserNoCache"`
	maxExpressionLength    int               `opt:"MaxExpressionLength" optgen:"default=DefaultMaxExpressionLength" optval:"positive"`
	maxKeys                int               `opt:"MaxKeys" optgen:"default=DefaultMaxKeys" optval:"positive"`
	maxFieldPathDepth      int               `opt:"MaxFieldPathDepth" optgen:"default=DefaultMaxFieldPathDepth" optval:"positive"`
	maxFieldNameLength     int               `opt:"MaxFieldNameLength" optgen:"default=DefaultMaxFieldNameLength" optval:"positive"`
	caseSensitiveDirection bool              `opt:"CaseSensitiveDirection"`
	allowArrayIndices      bool              `opt:"AllowArrayIndexPaths"`
	collector              metrics.Collector `opt:"ParserCollector" optgen:"notnil"`
}
