// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// regexCache caches compiled regular expressions to avoid recompilation
// on every evalMatches call. The sync.Map is ideal here: patterns are
// written once and read many times. Bounded to maxRegexCacheSize entries
// to prevent unbounded memory growth.
var regexCache sync.Map // map[string]*regexp.Regexp

// maxRegexCacheSize is the upper bound on cached regex entries. Once reached,
// new patterns are compiled but not cached.
const maxRegexCacheSize = 256

// Performance counters for the regex pattern cache.
var (
	regexHits   atomic.Uint64
	regexMisses atomic.Uint64
	regexSize   atomic.Int64
)

// RegexCacheStats holds a point-in-time snapshot of regex pattern cache
// performance counters. All values are cumulative since the last
// [ResetRegexCacheStats] call (or since process start).
type RegexCacheStats struct {
	// Hits is the number of lookups served from the cache.
	Hits uint64

	// Misses is the number of lookups that required pattern compilation.
	Misses uint64

	// Size is the current number of cached compiled patterns.
	Size int64
}

// HitRate returns the cache hit rate as a float64 in [0.0, 1.0].
// Returns 0 when there have been no lookups.
func (s RegexCacheStats) HitRate() float64 {
	total := s.Hits + s.Misses
	if total == 0 {
		return 0
	}
	return float64(s.Hits) / float64(total)
}

// TotalLookups returns the total number of getCompiledRegex calls tracked.
func (s RegexCacheStats) TotalLookups() uint64 {
	return s.Hits + s.Misses
}

// RegexCacheStatsSnapshot returns a point-in-time snapshot of the regex
// pattern cache performance counters.
func RegexCacheStatsSnapshot() RegexCacheStats {
	return RegexCacheStats{
		Hits:   regexHits.Load(),
		Misses: regexMisses.Load(),
		Size:   regexSize.Load(),
	}
}

// ResetRegexCacheStats zeroes all performance counters (hits, misses) without
// affecting the cached patterns or their compiled state.
func ResetRegexCacheStats() {
	regexHits.Store(0)
	regexMisses.Store(0)
}

func getCompiledRegex(pattern string) (*regexp.Regexp, error) {
	if cached, ok := regexCache.Load(pattern); ok {
		regexHits.Add(1)
		return cached.(*regexp.Regexp), nil //nolint:errcheck // type is guaranteed by store
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	if regexSize.Load() < maxRegexCacheSize {
		if _, loaded := regexCache.LoadOrStore(pattern, re); !loaded {
			regexSize.Add(1)
		}
	}

	regexMisses.Add(1)
	return re, nil
}

// Evaluator evaluates a filter AST against an in-memory map.
type Evaluator struct {
	config *TranslatorContext
	data   map[string]any
	depth  int
	ops    int
}

// NewEvaluator creates a new in-memory evaluator with the given
// options. Returns [ErrAllowlistRequired] when [WithUntrustedInput] is
// set without a non-empty [WithAllowedFields] — the misconfiguration
// is surfaced here rather than on the first Evaluate call.
func NewEvaluator(opts ...TranslatorOption) (*Evaluator, error) {
	ctx, err := NewTranslatorContext(opts...)
	if err != nil {
		return nil, err
	}
	return &Evaluator{config: ctx}, nil
}

// Evaluate returns true if data matches the filter node.
func (e *Evaluator) Evaluate(node Node, data map[string]any) (bool, error) {
	e.data = data
	e.depth = 0
	e.ops = 0
	result, err := node.Accept(e)
	if err != nil {
		return false, err
	}
	b, ok := result.(bool)
	if !ok {
		return false, coreerrs.Wrapf(ErrInvalidExpression, "expression must evaluate to bool, got %T", result)
	}
	return b, nil
}

// VisitLiteral returns the literal value.
func (e *Evaluator) VisitLiteral(n *LiteralNode) (any, error) {
	if err := e.checkOps(); err != nil {
		return nil, err
	}
	return n.Value, nil
}

// VisitIdent looks up the field in the data map, supporting dot notation.
func (e *Evaluator) VisitIdent(n *IdentNode) (any, error) {
	if err := e.checkOps(); err != nil {
		return nil, err
	}
	field := n.Name
	if !e.config.IsFieldAllowed(field) {
		return nil, coreerrs.Wrapf(ErrFieldNotAllowed, "%s", field)
	}
	mapped := e.config.ApplyFieldMapping(field)
	return lookupField(e.data, mapped), nil
}

// VisitBinaryOp evaluates binary operations.
func (e *Evaluator) VisitBinaryOp(n *BinaryOpNode) (any, error) {
	done, err := e.enterNode()
	if err != nil {
		return nil, err
	}
	defer done()

	switch n.Op {
	case OpAnd:
		return e.evalLogicalAnd(n.Left, n.Right)
	case OpOr:
		return e.evalLogicalOr(n.Left, n.Right)
	case OpIn:
		return e.evalIn(n.Left, n.Right)
	default:
		return e.evalComparison(n.Op, n.Left, n.Right)
	}
}

// VisitUnaryOp evaluates unary operations.
func (e *Evaluator) VisitUnaryOp(n *UnaryOpNode) (any, error) {
	done, err := e.enterNode()
	if err != nil {
		return nil, err
	}
	defer done()

	if n.Op != OpNot {
		return nil, coreerrs.Wrapf(ErrUnsupportedOperation, "unary operator %v", n.Op)
	}

	val, err := n.Operand.Accept(e)
	if err != nil {
		return nil, err
	}
	b, ok := val.(bool)
	if !ok {
		return nil, coreerrs.Wrapf(ErrInvalidExpression, "! requires bool operand, got %T", val)
	}
	return !b, nil
}

// VisitCall evaluates function calls.
func (e *Evaluator) VisitCall(n *CallNode) (any, error) {
	done, err := e.enterNode()
	if err != nil {
		return nil, err
	}
	defer done()

	switch n.Op {
	case OpContains:
		return e.evalStringFunc(n, strings.Contains)
	case OpStartsWith:
		return e.evalStringFunc(n, strings.HasPrefix)
	case OpEndsWith:
		return e.evalStringFunc(n, strings.HasSuffix)
	case OpMatches:
		return e.evalMatches(n)
	case OpHas, OpExists:
		return e.evalHas(n)
	case OpSize:
		return e.evalSize(n)
	case OpSubstring:
		return e.evalSubstring(n)
	default:
		return nil, coreerrs.Wrapf(ErrUnsupportedOperation, "function %v", n.Op)
	}
}

// VisitList evaluates a list literal.
func (e *Evaluator) VisitList(n *ListNode) (any, error) {
	if err := e.checkOps(); err != nil {
		return nil, err
	}
	result := make([]any, 0, len(n.Elements))
	for _, elem := range n.Elements {
		val, err := elem.Accept(e)
		if err != nil {
			return nil, err
		}
		result = append(result, val)
	}
	return result, nil
}

// evalLogicalAnd short-circuits on false.
func (e *Evaluator) evalLogicalAnd(left, right Node) (any, error) {
	return e.evalLogical(left, right, "&&", false)
}

// evalLogicalOr short-circuits on true.
func (e *Evaluator) evalLogicalOr(left, right Node) (any, error) {
	return e.evalLogical(left, right, "||", true)
}

// evalLogical evaluates a short-circuiting binary boolean operator. When the
// left operand equals shortCircuit, that value is returned without evaluating
// the right operand (false for &&, true for ||). opName labels the operator in
// error messages.
func (e *Evaluator) evalLogical(left, right Node, opName string, shortCircuit bool) (any, error) {
	lb, err := e.evalBoolOperand(left, opName)
	if err != nil {
		return nil, err
	}
	if lb == shortCircuit {
		return shortCircuit, nil
	}
	rb, err := e.evalBoolOperand(right, opName)
	if err != nil {
		return nil, err
	}
	return rb, nil
}

// evalBoolOperand evaluates n and asserts the result is a bool. opName labels
// the operator in the error message.
func (e *Evaluator) evalBoolOperand(n Node, opName string) (bool, error) {
	v, err := n.Accept(e)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, coreerrs.Wrapf(ErrInvalidExpression, "%s requires bool operands", opName)
	}
	return b, nil
}

// evalComparison evaluates comparison operators.
func (e *Evaluator) evalComparison(op Operator, left, right Node) (any, error) {
	if err := e.config.CheckComparison(left, right); err != nil {
		return nil, err
	}
	lv, err := left.Accept(e)
	if err != nil {
		return nil, err
	}
	rv, err := right.Accept(e)
	if err != nil {
		return nil, err
	}
	return compare(op, lv, rv)
}

// evalIn checks if the left value is in the right list. The schema
// check uses CheckLiteralKind directly because `in` has a fixed shape
// (ident on the left, list on the right) — the symmetric
// CheckComparison would also work but the asymmetry is intentional.
func (e *Evaluator) evalIn(left, right Node) (any, error) {
	if ident, ok := left.(*IdentNode); ok {
		if err := e.config.CheckLiteralKind(ident.Name, right); err != nil {
			return nil, err
		}
	}
	lv, err := left.Accept(e)
	if err != nil {
		return nil, err
	}
	rv, err := right.Accept(e)
	if err != nil {
		return nil, err
	}
	list, ok := rv.([]any)
	if !ok {
		return nil, coreerrs.Wrap(ErrInvalidExpression, "in requires a list on the right side")
	}
	for _, item := range list {
		if valuesEqual(lv, item) {
			return true, nil
		}
	}
	return false, nil
}

// stringCallArgs resolves the target and the single string argument shared by
// the string-valued call functions (contains/startsWith/endsWith/matches). A
// non-string target yields handled=false with no error so the caller can return
// false — matching the permissive semantics on non-string fields. label names
// the function in error messages.
func (e *Evaluator) stringCallArgs(n *CallNode, label string) (s, arg string, handled bool, err error) {
	target, err := n.Target.Accept(e)
	if err != nil {
		return "", "", false, err
	}
	s, ok := target.(string)
	if !ok {
		return "", "", false, nil
	}
	if len(n.Args) != 1 {
		return "", "", false, coreerrs.Wrapf(ErrInvalidExpression, "%s requires exactly 1 argument", label)
	}
	av, err := n.Args[0].Accept(e)
	if err != nil {
		return "", "", false, err
	}
	arg, ok = av.(string)
	if !ok {
		return "", "", false, coreerrs.Wrapf(ErrInvalidExpression, "%s argument must be a string", label)
	}
	return s, arg, true, nil
}

// evalStringFunc evaluates contains/startsWith/endsWith.
func (e *Evaluator) evalStringFunc(n *CallNode, fn func(string, string) bool) (any, error) {
	s, substr, handled, err := e.stringCallArgs(n, "string function")
	if err != nil {
		return nil, err
	}
	if !handled {
		return false, nil
	}
	return fn(s, substr), nil
}

// evalMatches evaluates regex matching.
func (e *Evaluator) evalMatches(n *CallNode) (any, error) {
	s, pattern, handled, err := e.stringCallArgs(n, "matches()")
	if err != nil {
		return nil, err
	}
	if !handled {
		return false, nil
	}
	if vErr := ValidateRegex(pattern, e.config.MaxRegexLength()); vErr != nil {
		return nil, vErr
	}
	re, mErr := getCompiledRegex(pattern)
	if mErr != nil {
		return nil, coreerrs.Wrapf(ErrInvalidExpression, "invalid regex: %v", mErr)
	}
	return re.MatchString(s), nil
}

// evalHas checks if a field exists (non-nil) in the data.
func (e *Evaluator) evalHas(n *CallNode) (any, error) {
	ident, ok := n.Target.(*IdentNode)
	if !ok {
		return nil, coreerrs.Wrap(ErrInvalidExpression, "has() requires an identifier")
	}
	field := ident.Name
	if !e.config.IsFieldAllowed(field) {
		return nil, coreerrs.Wrapf(ErrFieldNotAllowed, "%s", field)
	}
	mapped := e.config.ApplyFieldMapping(field)
	return fieldExists(e.data, mapped), nil
}

// evalSize returns the size of a string or slice.
func (e *Evaluator) evalSize(n *CallNode) (any, error) {
	target, err := n.Target.Accept(e)
	if err != nil {
		return nil, err
	}
	switch v := target.(type) {
	case string:
		return int64(len(v)), nil
	case []any:
		return int64(len(v)), nil
	default:
		return nil, coreerrs.Wrapf(ErrUnsupportedOperation, "size() not supported for %T", target)
	}
}

// evalSubstring returns target[start:end] (or target[start:] when end is
// omitted). Indices are 0-based and the range is half-open, matching the
// cel-go ext.Strings extension. start and end must be int64; both must lie
// within [0, len(runes)] and start must be <= end, otherwise an
// [ErrInvalidExpression] is returned.
//
// Indexing is over Unicode code points (runes), not bytes — same as
// cel-go ext.Strings — so a 3-rune Cyrillic prefix is substring(0, 3)
// regardless of the underlying UTF-8 byte length.
func (e *Evaluator) evalSubstring(n *CallNode) (any, error) {
	const (
		minSubstringArgs = 1
		maxSubstringArgs = 2
	)
	target, err := n.Target.Accept(e)
	if err != nil {
		return nil, err
	}
	s, ok := target.(string)
	if !ok {
		return nil, coreerrs.Wrapf(ErrInvalidExpression, "substring() target must be string, got %T", target)
	}
	if len(n.Args) < minSubstringArgs || len(n.Args) > maxSubstringArgs {
		return nil, coreerrs.Wrap(ErrInvalidExpression, "substring() requires 1 or 2 arguments")
	}
	start, err := e.evalIntArg(n.Args[0])
	if err != nil {
		return nil, err
	}
	// []rune(s) allocates 4×runeCount bytes per call. Fine for short
	// inputs (phone numbers, ids, names); rewrite as a
	// utf8.DecodeRuneInString walk if substring starts running on long
	// content fields under load.
	runes := []rune(s)
	end := int64(len(runes))
	if len(n.Args) == maxSubstringArgs {
		end, err = e.evalIntArg(n.Args[1])
		if err != nil {
			return nil, err
		}
	}
	if start < 0 || end < 0 || start > end || end > int64(len(runes)) {
		return nil, coreerrs.Wrapf(ErrInvalidExpression,
			"substring(%d, %d) out of range for string of length %d", start, end, len(runes))
	}
	return string(runes[start:end]), nil
}

// evalIntArg evaluates a Node and asserts the result is an int64. Used by
// integer-arg functions like substring.
func (e *Evaluator) evalIntArg(n Node) (int64, error) {
	v, err := n.Accept(e)
	if err != nil {
		return 0, err
	}
	i, ok := v.(int64)
	if !ok {
		return 0, coreerrs.Wrapf(ErrInvalidExpression, "argument must be int, got %T", v)
	}
	return i, nil
}

// walkField resolves a potentially dotted field path in the data map. The
// second result reports whether the full path exists; the first holds the value
// at that path (nil when the path is absent).
func walkField(data map[string]any, field string) (any, bool) {
	var current any = data
	for part := range strings.SplitSeq(field, ".") {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// lookupField resolves a potentially dotted field path in the data map,
// returning nil when the path is absent.
func lookupField(data map[string]any, field string) any {
	v, _ := walkField(data, field)
	return v
}

// fieldExists checks whether a dotted field path exists in the data map.
func fieldExists(data map[string]any, field string) bool {
	_, ok := walkField(data, field)
	return ok
}

// compare performs typed comparison.
func compare(op Operator, left, right any) (bool, error) {
	// Handle nil
	if left == nil || right == nil {
		return compareNil(op, left, right)
	}

	// Normalize numeric types for comparison
	ln, lok := toFloat64(left)
	rn, rok := toFloat64(right)
	if lok && rok {
		return compareOrdered(op, ln, rn)
	}

	// String comparison
	ls, lok := left.(string)
	rs, rok := right.(string)
	if lok && rok {
		return compareOrdered(op, ls, rs)
	}

	// Bool comparison (== and != only)
	lb, lok := left.(bool)
	rb, rok := right.(bool)
	if lok && rok {
		switch op {
		case OpEqual:
			return lb == rb, nil
		case OpNotEqual:
			return lb != rb, nil
		default:
			return false, coreerrs.Wrapf(ErrUnsupportedOperation, "cannot use %v on booleans", op)
		}
	}

	return false, coreerrs.Wrapf(ErrUnsupportedType, "cannot compare %T and %T", left, right)
}

func compareNil(op Operator, left, right any) (bool, error) {
	switch op {
	case OpEqual:
		return left == nil && right == nil, nil
	case OpNotEqual:
		return left != nil || right != nil, nil
	default:
		return false, coreerrs.Wrapf(ErrUnsupportedOperation, "cannot use %v with nil", op)
	}
}

type ordered interface {
	~int64 | ~float64 | ~string
}

func compareOrdered[T ordered](op Operator, a, b T) (bool, error) {
	switch op {
	case OpEqual:
		return a == b, nil
	case OpNotEqual:
		return a != b, nil
	case OpLT:
		return a < b, nil
	case OpLTE:
		return a <= b, nil
	case OpGT:
		return a > b, nil
	case OpGTE:
		return a >= b, nil
	default:
		return false, coreerrs.Wrapf(ErrUnsupportedOperation, "operator %v", op)
	}
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float64:
		return n, true
	case int32:
		return float64(n), true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

func valuesEqual(a, b any) bool {
	an, aok := toFloat64(a)
	bn, bok := toFloat64(b)
	if aok && bok {
		return an == bn
	}
	return a == b
}

// enterNode runs the per-node operation and depth guards shared by the
// compound visitor methods (binary, unary, call). On success it increments the
// recursion depth and returns a cleanup function the caller must defer to
// restore it.
func (e *Evaluator) enterNode() (func(), error) {
	if err := e.checkOps(); err != nil {
		return nil, err
	}
	if err := e.checkDepth(); err != nil {
		return nil, err
	}
	e.depth++
	return func() { e.depth-- }, nil
}

func (e *Evaluator) checkDepth() error {
	if e.depth >= e.config.MaxDepth() {
		return coreerrs.Wrapf(ErrMaxDepthExceeded, "depth %d exceeds maximum %d", e.depth, e.config.MaxDepth())
	}
	return nil
}

func (e *Evaluator) checkOps() error {
	e.ops++
	if e.ops > e.config.MaxOperations() {
		return coreerrs.Wrapf(ErrMaxOperationsExceeded, "operation count %d exceeds maximum %d", e.ops, e.config.MaxOperations())
	}
	return nil
}

// ValidateRegex validates a regex pattern for length and correctness.
// It enforces the given maxLength and compiles the pattern with Go's RE2 engine
// to reject patterns that could be problematic. This function is intended to be
// used by both the evaluator and translators for consistent regex validation.
func ValidateRegex(pattern string, maxLength int) error {
	if len(pattern) > maxLength {
		return coreerrs.Wrapf(ErrInvalidRegex, "pattern length %d exceeds maximum %d", len(pattern), maxLength)
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return coreerrs.Wrapf(ErrInvalidRegex, "%v", err)
	}
	return nil
}

// Ensure Evaluator implements Visitor.
var _ Visitor = (*Evaluator)(nil)
