// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/filter/internal/regexanchor"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// sizeFieldMarker is the internal key translateSize wraps a field in so the
// enclosing comparison can recognize a length expression. It never reaches a
// query: acceptPredicate rejects a document that still carries it.
const sizeFieldMarker = "__size_field__"

// Translator converts filter AST nodes to MongoDB bson.M filters.
//
// A Translator carries per-call state and is NOT safe for concurrent
// use. Construct one per goroutine, or guard it with a mutex.
type Translator struct {
	config *filter.TranslatorContext
	depth  filter.DepthGuard
}

// NewTranslator creates a new MongoDB translator with the given options.
// Returns [filter.ErrAllowlistRequired] when [filter.WithUntrustedInput]
// is set without a non-empty [filter.WithAllowedFields] — the
// misconfiguration is surfaced here rather than on the first Translate
// call.
func NewTranslator(opts ...filter.TranslatorOption) (*Translator, error) {
	ctx, err := filter.NewTranslatorContext(opts...)
	if err != nil {
		return nil, err
	}
	return &Translator{config: ctx, depth: filter.NewDepthGuard(ctx.MaxDepth())}, nil
}

// Translate converts a filter AST node to a MongoDB bson.M filter.
//
// A nil node translates to an empty filter document, which MongoDB reads
// as match-all. Returning that rather than dereferencing the nil lets a
// caller pass an absent filter straight through — the same contract the
// SQL and RediSearch translators offer.
func (t *Translator) Translate(node filter.Node) (bson.M, error) {
	if err := t.config.CheckLimits(node); err != nil {
		return nil, err
	}
	if node == nil {
		return bson.M{}, nil
	}

	t.depth.Reset()
	return t.acceptPredicate(node)
}

// acceptPredicate visits a node expected to produce a filter document.
//
// A bare identifier is treated as a boolean field test ({field: true}),
// matching the CEL semantics of using a field directly as a condition.
// The negated form has always been handled in translateNot; this is its
// counterpart, so `active` and `!active` are now symmetric wherever a
// predicate is expected — at the root and on either side of && / ||.
func (t *Translator) acceptPredicate(node filter.Node) (bson.M, error) {
	if ident, ok := node.(*filter.IdentNode); ok {
		field, err := t.getFieldName(ident)
		if err != nil {
			return nil, err
		}
		return bson.M{field: true}, nil
	}

	result, err := node.Accept(t)
	if err != nil {
		return nil, err
	}

	m, ok := result.(bson.M)
	if !ok {
		return nil, coreerrs.Wrapf(filter.ErrInvalidExpression, "expected bson.M, got %T", result)
	}
	// A length expression is an integer, not a predicate: on its own there is
	// nothing for the query to test. translateSize signals one by wrapping the
	// field in an internal marker for the enclosing comparison to unwrap, so
	// reaching the root with that marker still in place means no comparison
	// ever claimed it — and returning the document would send the marker key
	// itself to MongoDB as a field name, matching nothing and reporting no
	// error. Rejecting here matches the SQL translator, which turns the same
	// expression away with "size() outside a comparison".
	if _, isSizeMarker := m[sizeFieldMarker]; isSizeMarker {
		return nil, coreerrs.Wrap(filter.ErrUnsupportedOperation, "size() outside a comparison")
	}
	return m, nil
}

// VisitLiteral converts a literal value to its BSON representation.
func (t *Translator) VisitLiteral(n *filter.LiteralNode) (any, error) {
	return t.convertValue(n.Value)
}

// VisitIdent converts an identifier to a field reference.
func (t *Translator) VisitIdent(n *filter.IdentNode) (any, error) {
	field := n.Name
	if !t.config.IsFieldAllowed(field) {
		return nil, coreerrs.Wrapf(filter.ErrFieldNotAllowed, "%s", field)
	}
	return t.config.ApplyFieldMapping(field), nil
}

// VisitBinaryOp converts a binary operation to a MongoDB filter.
func (t *Translator) VisitBinaryOp(n *filter.BinaryOpNode) (any, error) {
	if err := t.depth.Enter(); err != nil {
		return nil, err
	}
	defer t.depth.Leave()

	switch n.Op {
	case filter.OpAnd:
		return t.translateLogical("$and", n.Left, n.Right)
	case filter.OpOr:
		return t.translateLogical("$or", n.Left, n.Right)
	case filter.OpIn:
		return t.translateIn(n.Left, n.Right)
	default:
		return t.translateComparison(n.Op, n.Left, n.Right)
	}
}

// VisitUnaryOp converts a unary operation to a MongoDB filter.
func (t *Translator) VisitUnaryOp(n *filter.UnaryOpNode) (any, error) {
	if err := t.depth.Enter(); err != nil {
		return nil, err
	}
	defer t.depth.Leave()

	if n.Op == filter.OpNot {
		return t.translateNot(n.Operand)
	}
	return nil, coreerrs.Wrapf(filter.ErrUnsupportedOperation, "unary operator %v", n.Op)
}

// VisitCall converts a function call to a MongoDB filter.
func (t *Translator) VisitCall(n *filter.CallNode) (any, error) {
	if err := t.depth.Enter(); err != nil {
		return nil, err
	}
	defer t.depth.Leave()

	switch n.Op {
	case filter.OpContains:
		return t.translateRegexOp(n.Target, n.Args, regexContains)
	case filter.OpStartsWith:
		return t.translateRegexOp(n.Target, n.Args, regexStartsWith)
	case filter.OpEndsWith:
		return t.translateRegexOp(n.Target, n.Args, regexEndsWith)
	case filter.OpMatches:
		return t.translateRegexOp(n.Target, n.Args, t.regexPassthrough)
	case filter.OpSize:
		return t.translateSize(n.Target)
	case filter.OpHas, filter.OpExists:
		return t.translateHas(n.Target)
	default:
		return nil, coreerrs.Wrapf(filter.ErrUnsupportedOperation, "function %v", n.Op)
	}
}

// VisitList converts a list to a slice of values.
func (t *Translator) VisitList(n *filter.ListNode) (any, error) {
	if err := t.depth.Enter(); err != nil {
		return nil, err
	}
	defer t.depth.Leave()
	return filter.VisitLiteralElements(t, n)
}

// regexTransform transforms a string argument into a regex pattern.
// Returns the pattern and an error if validation fails.
type regexTransform func(string) (string, error)

// The anchors are chosen for MongoDB's PCRE engine, which reads `$` as "the
// end, or just before a final newline": endsWith uses `\z`, the absolute end,
// so "abc\n" does not end with "abc". `^` needs no such care — without the m
// option it matches only at the start.
func regexContains(s string) (string, error)   { return quoteLiteral(s), nil }
func regexStartsWith(s string) (string, error) { return "^" + quoteLiteral(s), nil }
func regexEndsWith(s string) (string, error)   { return quoteLiteral(s) + `\z`, nil }

// quoteLiteral escapes s into a pattern matching it literally (see escapeNUL).
func quoteLiteral(s string) string {
	return escapeNUL(regexp.QuoteMeta(s))
}

// escapeNUL spells every raw NUL byte of a pattern as `\x00`: a BSON regex is a
// C string, so MongoDB rejects a raw NUL byte. A raw NUL in a pattern that
// passed RE2 validation is always a literal (RE2 rejects a backslash before
// it). Outside quoting, and in a character class, it becomes `\x00`; inside
// \Q…\E, where `\x00` would read as four literal characters, the quote is
// closed around it (`\E\x00\Q`), and an unterminated \Q is reopened the same
// way. The rewrite never changes what the pattern matches.
func escapeNUL(pattern string) string {
	if !strings.Contains(pattern, "\x00") {
		return pattern
	}
	var b strings.Builder
	b.Grow(len(pattern) + len(`\E\x00\Q`))
	quoted := false
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch {
		case quoted:
			switch {
			case c == 0:
				b.WriteString(`\E\x00\Q`)
			case c == '\\' && i+1 < len(pattern) && pattern[i+1] == 'E':
				b.WriteString(`\E`)
				i++
				quoted = false
			default:
				b.WriteByte(c)
			}
		case c == 0:
			b.WriteString(`\x00`)
		case c == '\\' && i+1 < len(pattern):
			// An escape: copy it whole so an escaped backslash is not read
			// as the start of another one; \Q opens quoting.
			b.WriteByte(c)
			b.WriteByte(pattern[i+1])
			quoted = pattern[i+1] == 'Q'
			i++
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// regexPassthrough validates a user-provided regex pattern before passing
// it to MongoDB. It delegates to [filter.ValidateRegex] using the
// translator-configured length cap from [filter.WithMaxRegexLength],
// falling back to [filter.DefaultMaxRegexLength] when the option was not
// set. Every `$` that RE2 reads as end of text is then rewritten to `\z`,
// so PCRE does not also match it before a final newline, and a raw NUL byte
// is spelled `\x00` (see escapeNUL).
//
// Security: validation compiles the pattern with Go's RE2 engine, which
// cannot backtrack — but MongoDB evaluates $regex with its own PCRE-family
// engine server-side, where a crafted pattern (e.g. nested quantifiers)
// CAN exhibit catastrophic backtracking against matching documents. The
// length cap bounds the blast radius but does not eliminate DB-side ReDoS.
// Expose matches() only to trusted callers, or tighten the cap via
// [filter.WithMaxRegexLength]; contains/startsWith/endsWith are safe
// (QuoteMeta produces literal-only patterns).
func (t *Translator) regexPassthrough(s string) (string, error) {
	maxLen := t.config.MaxRegexLength()
	if maxLen <= 0 {
		maxLen = filter.DefaultMaxRegexLength
	}
	if err := filter.ValidateRegex(s, maxLen); err != nil {
		return "", err
	}
	pattern, err := regexanchor.EndOfText(s)
	if err != nil {
		return "", err
	}
	return escapeNUL(pattern), nil
}

// translateComparison handles comparison operators.
func (t *Translator) translateComparison(op filter.Operator, left, right filter.Node) (bson.M, error) {
	// Check if left side is a size() call
	if call, ok := left.(*filter.CallNode); ok && call.Op == filter.OpSize {
		return t.translateSizeComparison(op, call, right)
	}

	// Type-check first: a mirrored `"lit" == field` is a type mismatch, not
	// a malformed field reference.
	if err := t.config.CheckComparison(left, right); err != nil {
		return nil, err
	}

	field, err := t.getFieldName(left)
	if err != nil {
		return nil, err
	}

	value, err := filter.AcceptLiteral(t, right)
	if err != nil {
		return nil, err
	}
	emitted, err := t.decimalValue(left, right, value)
	if err != nil {
		return nil, err
	}

	base, err := t.buildComparisonFilter(field, op, emitted)
	if err != nil {
		return nil, err
	}
	if kind, ok := t.zeroKind(left); ok {
		return absentAs(field, zeroMatchesComparison(op, kind, emitted), base), nil
	}
	return base, nil
}

// isDecimalField reports whether node is a field declared [filter.FieldKindDecimal].
func (t *Translator) isDecimalField(node filter.Node) bool {
	ident, ok := node.(*filter.IdentNode)
	return ok && t.config.FieldKind(ident.Name) == filter.FieldKindDecimal
}

// decimalValue converts a literal (value, produced by node) compared with a [filter.FieldKindDecimal] field to a BSON Decimal128,
// so MongoDB compares it with the stored Decimal128 exactly instead of through a binary double; a float literal is read from its
// source spelling ([filter.DecimalOperand]). Other fields and null keep the literal.
func (t *Translator) decimalValue(field, node filter.Node, value any) (any, error) {
	if value == nil || !t.isDecimalField(field) {
		return value, nil
	}
	d, err := filter.DecimalOperand(node, value)
	if err != nil {
		return nil, err
	}
	d128, ok := bson.ParseDecimal128FromBigInt(d.Coefficient(), int(d.Exponent()))
	if !ok {
		return nil, coreerrs.Wrapf(filter.ErrFieldTypeMismatch, "decimal %s does not fit Decimal128", d)
	}
	return d128, nil
}

// buildComparisonFilter creates a MongoDB comparison filter.
func (t *Translator) buildComparisonFilter(field string, op filter.Operator, value any) (bson.M, error) {
	switch op {
	case filter.OpEqual:
		return bson.M{field: value}, nil
	case filter.OpNotEqual:
		return bson.M{field: bson.M{"$ne": value}}, nil
	case filter.OpLT:
		return bson.M{field: bson.M{"$lt": value}}, nil
	case filter.OpLTE:
		return bson.M{field: bson.M{"$lte": value}}, nil
	case filter.OpGT:
		return bson.M{field: bson.M{"$gt": value}}, nil
	case filter.OpGTE:
		return bson.M{field: bson.M{"$gte": value}}, nil
	default:
		return nil, coreerrs.Wrapf(filter.ErrUnsupportedOperation, "comparison operator %v", op)
	}
}

// translateSizeComparison handles size() comparisons like tags.size() == 3.
//
// size() measures an array or a string. Arrays keep the $size forms, so an
// absent or null field behaves as it always has; a string is measured in code
// points with $strLenCP, as CEL defines size(). See stringLength for the
// guard that keeps $strLenCP off non-string values.
func (t *Translator) translateSizeComparison(op filter.Operator, call *filter.CallNode, right filter.Node) (bson.M, error) {
	if call.Target == nil {
		return nil, coreerrs.Wrap(filter.ErrInvalidExpression, "size() must be called as a method, e.g. field.size()")
	}

	field, err := t.getFieldName(call.Target)
	if err != nil {
		return nil, err
	}

	value, err := filter.AcceptLiteral(t, right)
	if err != nil {
		return nil, err
	}

	if err = sizeOperand(value); err != nil {
		return nil, err
	}
	kind, declared := t.zeroKind(call.Target)
	if !declared {
		return t.buildSizeFilter(field, op, value)
	}
	base, err := t.buildSizeFilter(field, op, value)
	if err != nil {
		return nil, err
	}
	return absentAs(field, zeroMatchesSize(op, kind, value), base), nil
}

// buildSizeFilter builds the size() comparison of field against value.
func (t *Translator) buildSizeFilter(field string, op filter.Operator, value any) (bson.M, error) {
	switch op {
	case filter.OpEqual:
		return bson.M{"$or": bson.A{
			bson.M{field: bson.M{"$size": value}},
			bson.M{"$expr": bson.M{"$eq": bson.A{stringLength(field), value}}},
		}}, nil
	case filter.OpNotEqual:
		return bson.M{"$and": bson.A{
			bson.M{field: bson.M{"$not": bson.M{"$size": value}}},
			bson.M{"$expr": bson.M{"$ne": bson.A{stringLength(field), value}}},
		}}, nil
	case filter.OpGT, filter.OpGTE, filter.OpLT, filter.OpLTE:
		return t.buildSizeExprFilter(field, op, value)
	default:
		return nil, coreerrs.Wrapf(filter.ErrUnsupportedOperation, "size comparison with operator %v", op)
	}
}

// stringLength is an aggregation expression for the code-point length of
// field when it holds a string, and null otherwise. $cond evaluates only the
// branch it takes, so $strLenCP — which fails the whole query on a non-string —
// never sees anything else.
func stringLength(field string) bson.M {
	return bson.M{"$cond": bson.A{
		bson.M{"$eq": bson.A{bson.M{"$type": "$" + field}, "string"}},
		bson.M{"$strLenCP": "$" + field},
		nil,
	}}
}

// buildSizeExprFilter builds a $expr filter for size ordering comparisons. An
// absent or null field counts as an empty array (length 0), an array by its
// element count and a string by its code points; any other value has no size
// and never matches.
func (t *Translator) buildSizeExprFilter(field string, op filter.Operator, value any) (bson.M, error) {
	opMap := map[filter.Operator]string{
		filter.OpGT:  "$gt",
		filter.OpGTE: "$gte",
		filter.OpLT:  "$lt",
		filter.OpLTE: "$lte",
	}

	mongoOp, ok := opMap[op]
	if !ok {
		return nil, coreerrs.Wrapf(filter.ErrUnsupportedOperation, "size comparison with operator %v", op)
	}

	// The length is spelled out twice rather than bound with $let: a $let
	// document carries plain keys (vars, in) that read like field references.
	orEmpty := bson.M{"$ifNull": bson.A{"$" + field, bson.A{}}}
	length := bson.M{"$cond": bson.A{
		bson.M{"$isArray": orEmpty},
		bson.M{"$size": orEmpty},
		stringLength(field),
	}}
	return bson.M{"$expr": bson.M{"$and": bson.A{
		bson.M{"$ne": bson.A{length, nil}},
		bson.M{mongoOp: bson.A{length, value}},
	}}}, nil
}

// translateLogical handles && and || operators.
func (t *Translator) translateLogical(mongoOp string, left, right filter.Node) (bson.M, error) {
	leftM, err := t.acceptPredicate(left)
	if err != nil {
		return nil, err
	}

	rightM, err := t.acceptPredicate(right)
	if err != nil {
		return nil, err
	}

	return bson.M{mongoOp: bson.A{leftM, rightM}}, nil
}

// translateNot handles the ! operator.
func (t *Translator) translateNot(operand filter.Node) (bson.M, error) {
	// For simple identifiers (like !active), treat as field != true
	if _, ok := operand.(*filter.IdentNode); ok {
		field, err := t.getFieldName(operand)
		if err != nil {
			return nil, err
		}
		return bson.M{field: bson.M{"$ne": true}}, nil
	}

	// For complex expressions, use $nor
	inner, err := t.acceptPredicate(operand)
	if err != nil {
		return nil, err
	}
	return bson.M{"$nor": bson.A{inner}}, nil
}

// translateIn handles the in operator.
func (t *Translator) translateIn(left, right filter.Node) (bson.M, error) {
	field, err := t.getFieldName(left)
	if err != nil {
		return nil, err
	}

	if ident, ok := left.(*filter.IdentNode); ok {
		if err = t.config.CheckLiteralKind(ident.Name, right); err != nil {
			return nil, err
		}
	}

	values, err := right.Accept(t)
	if err != nil {
		return nil, err
	}

	valuesSlice, ok := values.([]any)
	if !ok {
		return nil, coreerrs.Wrapf(filter.ErrInvalidExpression, "in operator requires a list, got %T", values)
	}
	emitted := valuesSlice
	if t.isDecimalField(left) {
		var elems []filter.Node
		if list, isList := right.(*filter.ListNode); isList && len(list.Elements) == len(valuesSlice) {
			elems = list.Elements
		}
		emitted = make([]any, len(valuesSlice))
		for i, v := range valuesSlice {
			var elem filter.Node
			if elems != nil {
				elem = elems[i]
			}
			if emitted[i], err = t.decimalValue(left, elem, v); err != nil {
				return nil, err
			}
		}
	}

	base := bson.M{field: bson.M{"$in": emitted}}
	if kind, declared := t.zeroKind(left); declared {
		return absentAs(field, zeroMatchesIn(kind, emitted), base), nil
	}
	return base, nil
}

// translateRegexOp handles string functions like contains, startsWith, endsWith, matches.
func (t *Translator) translateRegexOp(target filter.Node, args []filter.Node, transform regexTransform) (bson.M, error) {
	field, err := t.getFieldName(target)
	if err != nil {
		return nil, err
	}

	if len(args) != 1 {
		return nil, coreerrs.Wrap(filter.ErrInvalidExpression, "string function requires exactly 1 argument")
	}

	arg, err := filter.AcceptLiteral(t, args[0])
	if err != nil {
		return nil, err
	}

	pattern, ok := arg.(string)
	if !ok {
		return nil, coreerrs.Wrap(filter.ErrInvalidExpression, "string function argument must be a string")
	}

	regexPattern, err := transform(pattern)
	if err != nil {
		return nil, err
	}

	base := bson.M{field: bson.M{"$regex": regexPattern}}
	kind, declared := t.zeroKind(target)
	if !declared {
		return base, nil
	}
	zeroMatches, err := zeroMatchesRegex(kind, regexPattern)
	if err != nil {
		return nil, err
	}
	return absentAs(field, zeroMatches, base), nil
}

// translateSize handles the size() function.
func (t *Translator) translateSize(target filter.Node) (bson.M, error) {
	if target == nil {
		return nil, coreerrs.Wrap(filter.ErrInvalidExpression, "size() must be called as a method, e.g. field.size()")
	}

	field, err := t.getFieldName(target)
	if err != nil {
		return nil, err
	}

	// Return a special marker for parent binary op to handle
	return bson.M{sizeFieldMarker: field}, nil
}

// translateHas handles the has() function.
func (t *Translator) translateHas(target filter.Node) (bson.M, error) {
	field, err := t.getFieldName(target)
	if err != nil {
		return nil, err
	}

	// A declared field is never truly missing — its absence stands for the
	// zero value — so has() holds for every document.
	if _, declared := t.zeroKind(target); declared {
		return bson.M{}, nil
	}
	return bson.M{field: bson.M{"$exists": true}}, nil
}

// getFieldName extracts the field name from a node.
func (t *Translator) getFieldName(node filter.Node) (string, error) {
	if err := filter.CheckFieldReference(node); err != nil {
		return "", err
	}

	result, err := node.Accept(t)
	if err != nil {
		return "", err
	}

	// Handle size marker
	if m, ok := result.(bson.M); ok {
		if field, exists := m[sizeFieldMarker]; exists {
			if s, ok := field.(string); ok {
				return s, nil
			}
		}
	}

	field, ok := result.(string)
	if !ok {
		return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "expected field name, got %T", result)
	}
	return field, nil
}

// convertValue converts Go values to MongoDB-compatible values.
func (t *Translator) convertValue(v any) (any, error) {
	switch val := v.(type) {
	case nil, bool, int64, uint64, float64, string, []byte, time.Time:
		return val, nil
	default:
		return nil, coreerrs.Wrapf(filter.ErrUnsupportedType, "%T", v)
	}
}

// Ensure Translator implements filter.Visitor
var _ filter.Visitor = (*Translator)(nil)
