// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package orderby

import (
	"context"
	"strings"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/data/cache/lru"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Parser parses AIP-132 order_by strings into [Spec] values.
//
// The parser is safe for concurrent use after construction. An optional
// LRU cache (enabled by default; sized via [WithParserCacheSize], disabled
// via [WithParserNoCache]) memoizes parsed results keyed on the raw input
// string so repeated calls for the same expression are O(1).
type Parser struct {
	cfg     *parserOptions
	cache   lru.Cacher[string, Spec]
	metrics *orderByMetrics
}

// NewParser creates a new order_by parser.
func NewParser(opts ...ParserOption) (*Parser, error) {
	cfg := newParserOptions(opts...)

	p := &Parser{
		cfg:     cfg,
		metrics: newOrderByMetrics(cfg.collector),
	}

	if !cfg.noCache {
		cache, err := lru.NewCache[string, Spec](cfg.cacheSize)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "create order_by parser cache")
		}
		p.cache = cache
	}

	return p, nil
}

// Parse converts a raw order_by string into an [Spec] value. Empty or
// whitespace-only input parses to an empty Spec with no error,
// matching AIP-132's "no ordering specified" semantics.
//
// The returned Spec is always independent of the parser's internal
// LRU cache: callers may freely mutate Keys without corrupting a
// subsequent cache hit. The copy is cheap (a single slice allocation
// per call — Key fields are value types) and happens uniformly on every
// code path so behavior does not silently change when caching is
// disabled via [WithParserNoCache].
func (p *Parser) Parse(ctx context.Context, expression string) (Spec, error) {
	if len(expression) > p.cfg.maxExpressionLength {
		p.metrics.parseErrors.Inc()
		return Spec{}, coreerrs.Wrapf(ErrExpressionTooLong,
			"length %d exceeds maximum %d", len(expression), p.cfg.maxExpressionLength)
	}

	// parseDuration intentionally covers both cache hits and misses so
	// the histogram reflects the latency callers actually observe — a
	// cache miss buried under a fast P50 is precisely the signal an
	// operator wants when comparing cached vs no-cache parsers.
	stop := p.metrics.parseDuration.Start()
	defer stop()

	var (
		ob  Spec
		err error
	)
	if p.cache != nil {
		ob, err = p.cache.GetOrCompute(ctx, expression, func(_ context.Context) (Spec, error) {
			return p.parseInternal(expression)
		})
	} else {
		ob, err = p.parseInternal(expression)
	}
	if err != nil {
		p.metrics.parseErrors.Inc()
		return Spec{}, err
	}
	return ob.Clone(), nil
}

// MustParse parses an order_by string and panics if parsing fails. Use
// this for compile-time constants that should fail loudly during startup
// rather than at request time.
func (p *Parser) MustParse(expression string) Spec {
	return panics.MustResult(p.Parse(context.Background(), expression))
}

// parseInternal performs the actual parsing without caching or metrics.
//
// Error precedence is deterministic regardless of which side of an input
// violates which rule:
//  1. ErrExpressionTooLong   — checked by [Parser.Parse] before this runs.
//  2. ErrMaxKeysExceeded     — checked up-front from the comma count, so
//     an over-limit input that also contains empty clauses surfaces the
//     resource cap rather than the syntactic complaint.
//  3. ErrEmptyClause / per-clause syntactic errors — surfaced in
//     left-to-right order of the offending clause.
//  4. ErrDuplicateKey        — emitted on the second occurrence of a
//     field path; checked after the clause parses cleanly so a malformed
//     duplicate still reports its syntactic problem first.
func (p *Parser) parseInternal(expression string) (Spec, error) {
	trimmed := strings.TrimSpace(expression)
	if trimmed == "" {
		return Spec{}, nil
	}

	// commas+1 is an upper bound on the number of clauses (empty clauses
	// included). Checking it before per-clause validation makes the
	// maxKeys limit fire deterministically for any input that exceeds it,
	// even one that also contains empty clauses or syntax errors.
	if n := strings.Count(trimmed, ",") + 1; n > p.cfg.maxKeys {
		return Spec{}, coreerrs.Wrapf(ErrMaxKeysExceeded,
			"%d sort clauses exceeds maximum %d", n, p.cfg.maxKeys)
	}

	keys := make([]Key, 0, 4)
	// seen tracks already-encountered field paths so a second occurrence
	// of the same name surfaces ErrDuplicateKey instead of silently
	// producing a backend-specific quirk (Mongo lets the later entry win,
	// Meilisearch rejects the input, RediSearch trims to one key anyway).
	// Comparison is on Key.Name — the raw DSL form, before field mapping
	// — and case-sensitive, because AIP-132 identifiers are. Allocated
	// lazily on the second clause so single-key inputs stay alloc-free.
	var seen map[string]struct{}
	for clause := range strings.SplitSeq(trimmed, ",") {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			return Spec{}, coreerrs.Wrapf(ErrEmptyClause,
				"order_by %q contains an empty clause", expression)
		}

		key, err := p.parseClause(clause)
		if err != nil {
			return Spec{}, err
		}
		if len(keys) > 0 {
			if seen == nil {
				seen = make(map[string]struct{}, 4)
				seen[keys[0].Name] = struct{}{}
			}
			if _, dup := seen[key.Name]; dup {
				return Spec{}, coreerrs.Wrapf(ErrDuplicateKey,
					"field %q appears more than once", key.Name)
			}
			seen[key.Name] = struct{}{}
		}
		keys = append(keys, key)
	}

	return Spec{Keys: keys}, nil
}

// fieldAndDirectionTokens is the token count of a clause that carries an
// explicit direction (field + direction). A clause with the count below
// this is a bare field path that defaults to ascending; a higher count is
// malformed.
const fieldAndDirectionTokens = 2

// parseClause parses a single key clause: a field path optionally followed
// by an ascending/descending direction token.
func (p *Parser) parseClause(clause string) (Key, error) {
	tokens := strings.Fields(clause)
	if len(tokens) == 0 || len(tokens) > fieldAndDirectionTokens {
		return Key{}, coreerrs.Wrapf(ErrParseFailed,
			"clause %q: expected `field [asc|desc]`", clause)
	}

	fieldPath := tokens[0]
	dir := DirectionAscending
	if len(tokens) == fieldAndDirectionTokens {
		d, err := p.parseDirection(tokens[1])
		if err != nil {
			return Key{}, coreerrs.Wrapf(err, "clause %q", clause)
		}
		dir = d
	}

	if len(fieldPath) > p.cfg.maxFieldNameLength {
		return Key{}, coreerrs.Wrapf(ErrMaxFieldNameLengthExceeded,
			"field %q (%d bytes) exceeds maximum %d",
			fieldPath, len(fieldPath), p.cfg.maxFieldNameLength)
	}

	if err := p.validateFieldPath(fieldPath); err != nil {
		return Key{}, err
	}

	return Key{Name: fieldPath, Direction: dir}, nil
}

// parseDirection maps a direction token to a [Direction]. By default the
// match is case-insensitive (ASC/Asc/asc and DESC/Desc/desc all match);
// [WithCaseSensitiveDirection] restricts it to the lowercase tokens only.
func (p *Parser) parseDirection(token string) (Direction, error) {
	candidate := token
	if !p.cfg.caseSensitiveDirection {
		candidate = strings.ToLower(token)
	}
	switch candidate {
	case "asc":
		return DirectionAscending, nil
	case "desc":
		return DirectionDescending, nil
	default:
		return 0, coreerrs.Wrapf(ErrInvalidDirection, "%q", token)
	}
}

// validateFieldPath verifies that path is a dot-separated sequence of
// AIP-132 identifiers ([A-Za-z_][A-Za-z0-9_]*) and that the segment
// count stays within maxFieldPathDepth. The result is not retained —
// callers that need the parsed segments later call strings.Split on
// Key.Name themselves.
func (p *Parser) validateFieldPath(path string) error {
	segments := strings.Split(path, ".")
	if len(segments) > p.cfg.maxFieldPathDepth {
		return coreerrs.Wrapf(ErrMaxFieldPathDepthExceeded,
			"field %q has %d segments (max %d)",
			path, len(segments), p.cfg.maxFieldPathDepth)
	}
	for _, seg := range segments {
		valid := isIdent(seg) || (p.cfg.allowArrayIndices && isArrayIndex(seg))
		if !valid {
			return coreerrs.Wrapf(ErrInvalidFieldPath, "%q", path)
		}
	}
	return nil
}

// isIdent reports whether s matches the AIP-132 identifier grammar
// `[A-Za-z_][A-Za-z0-9_]*`. An empty string is not a valid identifier and
// double-dot field paths surface here as an empty segment.
func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		c := s[i]
		switch {
		case c == '_',
			c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z':
			// always valid
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

// isArrayIndex reports whether s is a non-empty all-digit segment such as
// "0" or "42". Mongo and JSONPath both accept leading-zero forms ("001"),
// so we do not reject them. Enabled only when [WithAllowArrayIndexPaths]
// is configured on the parser.
func isArrayIndex(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
