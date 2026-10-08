// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package projection

import (
	"context"
	"slices"
	"strings"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/data/cache/lru"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// allFields is the AIP-157 token that selects every field.
const allFields = "*"

// Parser parses fields expressions into [Spec] values.
//
// The parser is safe for concurrent use after construction. An optional LRU
// cache (enabled by default; sized via [WithParserCacheSize], disabled via
// [WithParserNoCache]) memoizes parsed results keyed on the raw input.
type Parser struct {
	cfg     *parserOptions
	cache   lru.Cacher[string, Spec]
	metrics *projectionMetrics
}

// NewParser creates a new fields parser.
func NewParser(opts ...ParserOption) (*Parser, error) {
	cfg := newParserOptions(opts...)

	p := &Parser{
		cfg:     cfg,
		metrics: newProjectionMetrics(cfg.collector),
	}

	if !cfg.noCache {
		cache, err := lru.NewCache[string, Spec](cfg.cacheSize)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "create fields parser cache")
		}
		p.cache = cache
	}

	return p, nil
}

// Parse converts a raw fields expression — comma-separated dotted paths —
// into a [Spec]. Empty, whitespace-only and "*" input parse to an empty Spec
// ("all authorized fields") with no error, matching AIP-157.
//
// The returned Spec never aliases the parser's cache: callers may mutate it
// freely.
func (p *Parser) Parse(ctx context.Context, expression string) (Spec, error) {
	if len(expression) > p.cfg.maxExpressionLength {
		p.metrics.parseErrors.Inc()
		return Spec{}, coreerrs.Wrapf(ErrExpressionTooLong,
			"length %d exceeds maximum %d", len(expression), p.cfg.maxExpressionLength)
	}

	stop := p.metrics.parseDuration.Start()
	defer stop()

	var (
		spec Spec
		err  error
	)
	if p.cache != nil {
		spec, err = p.cache.GetOrCompute(ctx, expression, func(_ context.Context) (Spec, error) {
			return p.parseInternal(expression)
		})
	} else {
		spec, err = p.parseInternal(expression)
	}
	if err != nil {
		p.metrics.parseErrors.Inc()
		return Spec{}, err
	}
	return spec.Clone(), nil
}

// ParsePaths parses an already-split path list, such as
// fieldmaskpb.FieldMask.Paths or the result of a fieldmask read extractor.
// An empty list, or a single "*", selects all authorized fields. The count
// and total-length limits apply to the raw list before any other work, so an
// oversized list is rejected without being joined.
func (p *Parser) ParsePaths(ctx context.Context, paths []string) (Spec, error) {
	if len(paths) == 0 {
		return Spec{}, nil
	}
	if len(paths) > p.cfg.maxPaths {
		p.metrics.parseErrors.Inc()
		return Spec{}, coreerrs.Wrapf(ErrMaxPathsExceeded,
			"%d paths exceeds maximum %d", len(paths), p.cfg.maxPaths)
	}
	total := len(paths) - 1 // the commas of the joined form
	for _, path := range paths {
		total += len(path)
	}
	if total > p.cfg.maxExpressionLength {
		p.metrics.parseErrors.Inc()
		return Spec{}, coreerrs.Wrapf(ErrExpressionTooLong,
			"length %d exceeds maximum %d", total, p.cfg.maxExpressionLength)
	}
	for _, path := range paths {
		// A comma inside one element would be split into several paths by
		// the joined form, so a single invalid element could parse — and be
		// cached — as a valid list. An empty element would silently vanish
		// when it is the only one.
		switch {
		case strings.Contains(path, ","):
			p.metrics.parseErrors.Inc()
			return Spec{}, coreerrs.Wrapf(ErrInvalidFieldPath, "%q", path)
		case strings.TrimSpace(path) == "":
			p.metrics.parseErrors.Inc()
			return Spec{}, coreerrs.Wrapf(ErrEmptyClause, "paths %q contain an empty path", paths)
		}
	}
	return p.Parse(ctx, strings.Join(paths, ","))
}

// MustParse parses a fields expression and panics if parsing fails. Use it
// for constants that should fail loudly during startup.
func (p *Parser) MustParse(expression string) Spec {
	return panics.MustResult(p.Parse(context.Background(), expression))
}

// parseInternal performs the actual parsing without caching or metrics.
//
// Error precedence is deterministic:
//  1. ErrExpressionTooLong — checked by [Parser.Parse] before this runs.
//  2. ErrMaxPathsExceeded  — checked up-front from the comma count, before
//     deduplication, so repeating one path cannot bypass the limit.
//  3. Per-path errors      — in left-to-right order of the offending path.
func (p *Parser) parseInternal(expression string) (Spec, error) {
	trimmed := strings.TrimSpace(expression)
	if trimmed == "" || trimmed == allFields {
		return Spec{}, nil
	}

	if n := strings.Count(trimmed, ",") + 1; n > p.cfg.maxPaths {
		return Spec{}, coreerrs.Wrapf(ErrMaxPathsExceeded,
			"%d paths exceeds maximum %d", n, p.cfg.maxPaths)
	}

	paths := make([]string, 0, 8)
	for clause := range strings.SplitSeq(trimmed, ",") {
		path := strings.TrimSpace(clause)
		if path == "" {
			return Spec{}, coreerrs.Wrapf(ErrEmptyClause,
				"fields %q contain an empty path", expression)
		}
		if err := p.validatePath(path); err != nil {
			return Spec{}, err
		}
		paths = append(paths, path)
	}

	// A field list is a set: duplicates are harmless and dropped. Sorting
	// makes the Spec independent of the order the client wrote.
	slices.Sort(paths)
	return Spec{Paths: slices.Compact(paths)}, nil
}

// validatePath checks the length and depth limits and the segment grammar.
func (p *Parser) validatePath(path string) error {
	if len(path) > p.cfg.maxFieldNameLength {
		return coreerrs.Wrapf(ErrMaxFieldNameLengthExceeded,
			"field %q (%d bytes) exceeds maximum %d", path, len(path), p.cfg.maxFieldNameLength)
	}
	if depth := strings.Count(path, ".") + 1; depth > p.cfg.maxFieldPathDepth {
		return coreerrs.Wrapf(ErrMaxFieldPathDepthExceeded,
			"field %q has %d segments (max %d)", path, depth, p.cfg.maxFieldPathDepth)
	}
	return checkPath(path)
}

// checkPath verifies that path is a dot-separated sequence of identifiers
// ([A-Za-z_][A-Za-z0-9_]*). AIP-161 syntax this package does not support —
// wildcards, backtick-quoted keys and numeric segments — is reported as
// [ErrUnsupportedPath] so clients learn the mask is valid but not served.
func checkPath(path string) error {
	for seg := range strings.SplitSeq(path, ".") {
		switch {
		case isIdent(seg):
		case seg == allFields, strings.Contains(seg, "`"), isNumeric(seg):
			return coreerrs.Wrapf(ErrUnsupportedPath, "%q", path)
		default:
			return coreerrs.Wrapf(ErrInvalidFieldPath, "%q", path)
		}
	}
	return nil
}

// isIdent reports whether s matches `[A-Za-z_][A-Za-z0-9_]*`.
func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		c := s[i]
		switch {
		case c == '_', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// isNumeric reports whether s is a non-empty all-digit segment — an array
// index or an integer map key, which this package does not address.
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
