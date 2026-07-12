// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/data/filter"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Translator converts filter AST nodes to MongoDB bson.M filters.
type Translator struct {
	config *filter.TranslatorContext
	depth  int
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
	return &Translator{config: ctx}, nil
}

// Translate converts a filter AST node to a MongoDB bson.M filter.
func (t *Translator) Translate(node filter.Node) (bson.M, error) {
	t.depth = 0
	result, err := node.Accept(t)
	if err != nil {
		return nil, err
	}

	m, ok := result.(bson.M)
	if !ok {
		return nil, coreerrs.Wrapf(filter.ErrInvalidExpression, "expected bson.M, got %T", result)
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
	if err := t.checkDepth(); err != nil {
		return nil, err
	}
	t.depth++
	defer func() { t.depth-- }()

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
	if err := t.checkDepth(); err != nil {
		return nil, err
	}
	t.depth++
	defer func() { t.depth-- }()

	if n.Op == filter.OpNot {
		return t.translateNot(n.Operand)
	}
	return nil, coreerrs.Wrapf(filter.ErrUnsupportedOperation, "unary operator %v", n.Op)
}

// VisitCall converts a function call to a MongoDB filter.
func (t *Translator) VisitCall(n *filter.CallNode) (any, error) {
	if err := t.checkDepth(); err != nil {
		return nil, err
	}
	t.depth++
	defer func() { t.depth-- }()

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
	result := make([]any, 0, len(n.Elements))
	for _, elem := range n.Elements {
		val, err := elem.Accept(t)
		if err != nil {
			return nil, err
		}
		result = append(result, val)
	}
	return result, nil
}

// regexTransform transforms a string argument into a regex pattern.
// Returns the pattern and an error if validation fails.
type regexTransform func(string) (string, error)

func regexContains(s string) (string, error)   { return regexp.QuoteMeta(s), nil }
func regexStartsWith(s string) (string, error) { return "^" + regexp.QuoteMeta(s), nil }
func regexEndsWith(s string) (string, error)   { return regexp.QuoteMeta(s) + "$", nil }

// regexPassthrough validates a user-provided regex pattern before passing
// it to MongoDB. It delegates to [filter.ValidateRegex] using the
// translator-configured length cap from [filter.WithMaxRegexLength],
// falling back to [filter.DefaultMaxRegexLength] when the option was not
// set.
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
	return s, nil
}

// translateComparison handles comparison operators.
func (t *Translator) translateComparison(op filter.Operator, left, right filter.Node) (bson.M, error) {
	// Check if left side is a size() call
	if call, ok := left.(*filter.CallNode); ok && call.Op == filter.OpSize {
		return t.translateSizeComparison(op, call, right)
	}

	field, err := t.getFieldName(left)
	if err != nil {
		return nil, err
	}

	if err = t.config.CheckComparison(left, right); err != nil {
		return nil, err
	}

	value, err := right.Accept(t)
	if err != nil {
		return nil, err
	}

	return t.buildComparisonFilter(field, op, value)
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
func (t *Translator) translateSizeComparison(op filter.Operator, call *filter.CallNode, right filter.Node) (bson.M, error) {
	if call.Target == nil {
		return nil, coreerrs.Wrap(filter.ErrInvalidExpression, "size() must be called as a method, e.g. field.size()")
	}

	field, err := t.getFieldName(call.Target)
	if err != nil {
		return nil, err
	}

	value, err := right.Accept(t)
	if err != nil {
		return nil, err
	}

	switch op {
	case filter.OpEqual:
		return bson.M{field: bson.M{"$size": value}}, nil
	case filter.OpNotEqual:
		return bson.M{field: bson.M{"$not": bson.M{"$size": value}}}, nil
	case filter.OpGT, filter.OpGTE, filter.OpLT, filter.OpLTE:
		return t.buildSizeExprFilter(field, op, value)
	default:
		return nil, coreerrs.Wrapf(filter.ErrUnsupportedOperation, "size comparison with operator %v", op)
	}
}

// buildSizeExprFilter builds a $expr filter for size comparisons.
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

	return bson.M{
		"$expr": bson.M{
			mongoOp: bson.A{
				bson.M{"$size": bson.M{"$ifNull": bson.A{"$" + field, bson.A{}}}},
				value,
			},
		},
	}, nil
}

// translateLogical handles && and || operators.
func (t *Translator) translateLogical(mongoOp string, left, right filter.Node) (bson.M, error) {
	leftFilter, err := left.Accept(t)
	if err != nil {
		return nil, err
	}
	leftM, ok := leftFilter.(bson.M)
	if !ok {
		return nil, coreerrs.Wrapf(filter.ErrInvalidExpression, "expected bson.M for logical operand, got %T", leftFilter)
	}

	rightFilter, err := right.Accept(t)
	if err != nil {
		return nil, err
	}
	rightM, ok := rightFilter.(bson.M)
	if !ok {
		return nil, coreerrs.Wrapf(filter.ErrInvalidExpression, "expected bson.M for logical operand, got %T", rightFilter)
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
	inner, err := operand.Accept(t)
	if err != nil {
		return nil, err
	}
	innerM, ok := inner.(bson.M)
	if !ok {
		return nil, coreerrs.Wrapf(filter.ErrInvalidExpression, "expected bson.M for not operand, got %T", inner)
	}

	return bson.M{"$nor": bson.A{innerM}}, nil
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

	return bson.M{field: bson.M{"$in": valuesSlice}}, nil
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

	arg, err := args[0].Accept(t)
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

	return bson.M{field: bson.M{"$regex": regexPattern}}, nil
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
	return bson.M{"__size_field__": field}, nil
}

// translateHas handles the has() function.
func (t *Translator) translateHas(target filter.Node) (bson.M, error) {
	field, err := t.getFieldName(target)
	if err != nil {
		return nil, err
	}

	return bson.M{field: bson.M{"$exists": true}}, nil
}

// getFieldName extracts the field name from a node.
func (t *Translator) getFieldName(node filter.Node) (string, error) {
	if node == nil {
		return "", coreerrs.Wrap(filter.ErrInvalidExpression, "missing field reference")
	}

	result, err := node.Accept(t)
	if err != nil {
		return "", err
	}

	// Handle size marker
	if m, ok := result.(bson.M); ok {
		if field, exists := m["__size_field__"]; exists {
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

// checkDepth verifies we haven't exceeded maximum nesting depth.
func (t *Translator) checkDepth() error {
	if t.depth >= t.config.MaxDepth() {
		return coreerrs.Wrapf(filter.ErrMaxDepthExceeded, "depth %d exceeds maximum %d", t.depth, t.config.MaxDepth())
	}
	return nil
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
