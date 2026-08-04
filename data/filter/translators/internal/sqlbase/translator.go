// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqlbase

import (
	"strings"
	"time"

	"github.com/altessa-s/go-atlas/data/filter"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// MatchAll is the clause emitted for a nil AST — a filter that selects
// every row. Returning a valid predicate rather than an empty string
// keeps `"... WHERE " + clause` concatenation safe at the call site.
const MatchAll = "1 = 1"

// MatchNone is the clause emitted for `field in []`. CEL evaluates
// membership in an empty list to false, and every SQL dialect here
// rejects the literal `IN ()` as a syntax error.
const MatchNone = "1 = 0"

// Translator walks a filter AST and renders it as a SQL WHERE clause,
// deferring every backend-specific decision to a [Dialect].
//
// Two output modes share one walk. [Translator.Translate] emits bind
// placeholders and collects the values into a separate argument slice;
// [Translator.TranslateInline] emits self-contained SQL with every
// literal rendered in place.
//
// A Translator carries per-call state (nesting depth, collected
// arguments) and is therefore NOT safe for concurrent use.
type Translator struct {
	config  *filter.TranslatorContext
	dialect Dialect
	args    []any
	depth   int
	inline  bool
}

// New builds a translator for the given dialect. It returns
// [filter.ErrAllowlistRequired] when [filter.WithUntrustedInput] is set
// without a non-empty [filter.WithAllowedFields], so the
// misconfiguration surfaces at construction rather than on the first
// Translate call.
func New(dialect Dialect, opts ...filter.TranslatorOption) (*Translator, error) {
	ctx, err := filter.NewTranslatorContext(opts...)
	if err != nil {
		return nil, err
	}
	return &Translator{config: ctx, dialect: dialect}, nil
}

// Translate converts a filter AST node to a parameterized WHERE clause.
// Every literal from the expression is replaced by a bind placeholder
// and returned in args, positionally aligned with its placeholder — no
// value from the filter reaches the SQL text, so the clause cannot be
// made to carry an injection regardless of the input.
//
// A nil node translates to [MatchAll] and an empty argument list.
func (t *Translator) Translate(node filter.Node) (string, []any, error) {
	sql, err := t.run(node, false)
	args := t.args
	t.args = nil
	if err != nil {
		return "", nil, err
	}
	return sql, args, nil
}

// TranslateInline converts a filter AST node to a self-contained WHERE
// clause with every literal rendered in place. Use it where a
// parameterized query is not an option — view definitions, generated
// DDL, logging, debugging.
//
// Prefer [Translator.Translate] for anything driven by request data:
// inline rendering puts caller-controlled values into the SQL text, and
// its safety rests entirely on the dialect's quoting rather than on the
// protocol-level separation the placeholder form gives for free.
func (t *Translator) TranslateInline(node filter.Node) (string, error) {
	sql, err := t.run(node, true)
	t.args = nil
	return sql, err
}

// run resets the per-call state and walks the AST in the requested mode.
func (t *Translator) run(node filter.Node, inline bool) (string, error) {
	t.depth = 0
	t.inline = inline
	t.args = nil

	if node == nil {
		return MatchAll, nil
	}
	return t.acceptPredicate(node)
}

// VisitLiteral validates a literal value and returns it unchanged. The
// rendering decision — placeholder or inline text — belongs to the
// operand that consumes the value, so it is deferred to [Translator.value].
func (t *Translator) VisitLiteral(n *filter.LiteralNode) (any, error) {
	if n.Value != nil && !supportedValue(n.Value) {
		return nil, coreerrs.Wrapf(filter.ErrUnsupportedType, "%T", n.Value)
	}
	return n.Value, nil
}

// VisitIdent converts an identifier to a quoted column reference. The
// name is checked against the allow-list and run through the field
// mapping before the dialect validates and quotes it, so a mapped name
// can never break out of its quotes.
func (t *Translator) VisitIdent(n *filter.IdentNode) (any, error) {
	if !t.config.IsFieldAllowed(n.Name) {
		return nil, coreerrs.Wrapf(filter.ErrFieldNotAllowed, "%s", n.Name)
	}
	return t.dialect.QuoteIdent(t.config.ApplyFieldMapping(n.Name))
}

// VisitBinaryOp converts a binary operation to a SQL clause.
func (t *Translator) VisitBinaryOp(n *filter.BinaryOpNode) (any, error) {
	if err := t.checkDepth(); err != nil {
		return nil, err
	}
	t.depth++
	defer func() { t.depth-- }()

	switch n.Op {
	case filter.OpAnd:
		return t.translateLogical("AND", n.Left, n.Right)
	case filter.OpOr:
		return t.translateLogical("OR", n.Left, n.Right)
	case filter.OpIn:
		return t.translateIn(n.Left, n.Right)
	default:
		return t.translateComparison(n.Op, n.Left, n.Right)
	}
}

// VisitUnaryOp converts a unary operation to a SQL clause.
func (t *Translator) VisitUnaryOp(n *filter.UnaryOpNode) (any, error) {
	if err := t.checkDepth(); err != nil {
		return nil, err
	}
	t.depth++
	defer func() { t.depth-- }()

	if n.Op == filter.OpNot {
		inner, err := t.acceptPredicate(n.Operand)
		if err != nil {
			return nil, err
		}
		return "NOT (" + inner + ")", nil
	}
	return nil, coreerrs.Wrapf(filter.ErrUnsupportedOperation, "unary operator %v", n.Op)
}

// VisitCall converts a function call to a SQL clause.
func (t *Translator) VisitCall(n *filter.CallNode) (any, error) {
	if err := t.checkDepth(); err != nil {
		return nil, err
	}
	t.depth++
	defer func() { t.depth-- }()

	switch n.Op {
	case filter.OpContains, filter.OpStartsWith, filter.OpEndsWith, filter.OpMatches:
		return t.translateStringPredicate(n)
	case filter.OpHas, filter.OpExists:
		return t.translateExists(n.Target)
	case filter.OpSize:
		// A length expression is an integer, not a predicate. It is
		// rendered by operandSQL when it appears inside a comparison; on
		// its own there is nothing for WHERE to test.
		return nil, coreerrs.Wrap(filter.ErrUnsupportedOperation, "size() outside a comparison")
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

// translateComparison handles the comparison operators, including the
// `IS NULL` / `IS NOT NULL` forms a null literal calls for.
func (t *Translator) translateComparison(op filter.Operator, left, right filter.Node) (string, error) {
	if err := t.config.CheckComparison(left, right); err != nil {
		return "", err
	}

	if isNullLiteral(right) {
		return t.translateNullComparison(op, left)
	}
	if isNullLiteral(left) {
		return t.translateNullComparison(op, right)
	}

	sqlOp, err := comparisonOperator(op)
	if err != nil {
		return "", err
	}

	// Operands are rendered left-to-right so that any placeholders they
	// emit line up with the order of t.args.
	lhs, err := t.operandSQL(left)
	if err != nil {
		return "", err
	}
	rhs, err := t.operandSQL(right)
	if err != nil {
		return "", err
	}

	return lhs + " " + sqlOp + " " + rhs, nil
}

// translateNullComparison renders `expr IS [NOT] NULL`.
func (t *Translator) translateNullComparison(op filter.Operator, other filter.Node) (string, error) {
	expr, err := t.operandSQL(other)
	if err != nil {
		return "", err
	}

	switch op {
	case filter.OpEqual:
		return expr + " IS NULL", nil
	case filter.OpNotEqual:
		return expr + " IS NOT NULL", nil
	default:
		return "", coreerrs.Wrapf(filter.ErrUnsupportedOperation, "comparison %v with null", op)
	}
}

// translateLogical handles the && and || operators.
func (t *Translator) translateLogical(sqlOp string, left, right filter.Node) (string, error) {
	lhs, err := t.acceptPredicate(left)
	if err != nil {
		return "", err
	}
	rhs, err := t.acceptPredicate(right)
	if err != nil {
		return "", err
	}
	return "(" + lhs + ") " + sqlOp + " (" + rhs + ")", nil
}

// translateIn handles the in operator.
func (t *Translator) translateIn(left, right filter.Node) (string, error) {
	if ident, ok := left.(*filter.IdentNode); ok {
		if err := t.config.CheckLiteralKind(ident.Name, right); err != nil {
			return "", err
		}
	}

	list, ok := right.(*filter.ListNode)
	if !ok {
		return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "in operator requires a list, got %T", right)
	}

	// The left operand is rendered even for an empty list so that the
	// allow-list and identifier checks still run on it; the placeholders
	// it may have emitted are rolled back before the constant is
	// returned, keeping numbered dialects consistent.
	argsBefore := len(t.args)
	expr, err := t.operandSQL(left)
	if err != nil {
		return "", err
	}
	if len(list.Elements) == 0 {
		t.args = t.args[:argsBefore]
		return MatchNone, nil
	}

	// Routed through VisitList rather than walking the elements here so
	// that the Visitor method is the single implementation. It is public
	// surface — the walker is embedded in the dialect translators, which
	// therefore satisfy filter.Visitor — and an implementation nothing
	// calls is one nothing tests either.
	values, err := t.VisitList(list)
	if err != nil {
		return "", err
	}

	elements, ok := values.([]any)
	if !ok {
		return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "expected list elements, got %T", values)
	}

	rendered := make([]string, 0, len(elements))
	for _, value := range elements {
		sql, err := t.value(value)
		if err != nil {
			return "", err
		}
		rendered = append(rendered, sql)
	}

	return expr + " IN (" + strings.Join(rendered, ", ") + ")", nil
}

// translateStringPredicate handles contains, startsWith, endsWith and
// matches. The needle is passed to the dialect raw, together with
// [Translator.value], so the dialect can bind it as many times as its
// rendering needs.
func (t *Translator) translateStringPredicate(n *filter.CallNode) (string, error) {
	col, err := t.column(n.Target)
	if err != nil {
		return "", err
	}

	if len(n.Args) != 1 {
		return "", coreerrs.Wrap(filter.ErrInvalidExpression, "string function requires exactly 1 argument")
	}

	arg, err := n.Args[0].Accept(t)
	if err != nil {
		return "", err
	}

	needle, ok := arg.(string)
	if !ok {
		return "", coreerrs.Wrap(filter.ErrInvalidExpression, "string function argument must be a string")
	}

	if n.Op == filter.OpMatches {
		if err = t.validateRegex(needle); err != nil {
			return "", err
		}
	}

	return t.dialect.StringPredicate(n.Op, col, needle, t.value)
}

// translateExists handles has() / exists(). A SQL row always carries
// every column of its table, so the closest available meaning is "the
// nullable column holds a value".
func (t *Translator) translateExists(target filter.Node) (string, error) {
	col, err := t.column(target)
	if err != nil {
		return "", err
	}
	return col + " IS NOT NULL", nil
}

// operandSQL renders one side of a comparison: a column reference, a
// size expression over one, or a literal.
func (t *Translator) operandSQL(node filter.Node) (string, error) {
	switch n := node.(type) {
	case *filter.IdentNode:
		return t.column(n)
	case *filter.LiteralNode:
		value, err := t.VisitLiteral(n)
		if err != nil {
			return "", err
		}
		return t.value(value)
	case *filter.CallNode:
		if n.Op == filter.OpSize {
			return t.sizeSQL(n)
		}
	}
	return "", coreerrs.Wrapf(filter.ErrUnsupportedOperation, "%T as a comparison operand", node)
}

// sizeSQL renders a size() call through the dialect. Both call forms the
// parser produces — `field.size()` and `size(field)` — are accepted.
func (t *Translator) sizeSQL(n *filter.CallNode) (string, error) {
	target := n.Target
	if target == nil && len(n.Args) == 1 {
		target = n.Args[0]
	}

	col, err := t.column(target)
	if err != nil {
		return "", err
	}
	return t.dialect.SizeExpr(col), nil
}

// column resolves a node that must be a field reference into its quoted
// column name.
func (t *Translator) column(node filter.Node) (string, error) {
	ident, ok := node.(*filter.IdentNode)
	if !ok {
		return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "expected field reference, got %T", node)
	}

	result, err := t.VisitIdent(ident)
	if err != nil {
		return "", err
	}
	col, ok := result.(string)
	if !ok {
		return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "dialect returned %T for a column name", result)
	}
	return col, nil
}

// acceptPredicate visits a node expected to produce a boolean SQL
// clause. A bare identifier is treated as a boolean column test
// (`col = TRUE`), matching the CEL semantics of using a field directly
// as a condition.
func (t *Translator) acceptPredicate(node filter.Node) (string, error) {
	if ident, ok := node.(*filter.IdentNode); ok {
		col, err := t.column(ident)
		if err != nil {
			return "", err
		}
		// The operand is a constant, never caller data, so it is
		// rendered inline in both modes rather than burning a bind slot.
		lit, err := t.dialect.FormatLiteral(true)
		if err != nil {
			return "", err
		}
		return col + " = " + lit, nil
	}

	result, err := node.Accept(t)
	if err != nil {
		return "", err
	}
	clause, ok := result.(string)
	if !ok {
		return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "expected filter clause, got %T", result)
	}
	return clause, nil
}

// value renders a literal for the active mode: a bind placeholder with
// the value pushed onto the argument slice, or inline SQL text. A null
// literal renders as NULL in both modes — there is nothing to bind.
func (t *Translator) value(v any) (string, error) {
	if v == nil {
		return "NULL", nil
	}
	if t.inline {
		return t.dialect.FormatLiteral(v)
	}
	if !supportedValue(v) {
		return "", coreerrs.Wrapf(filter.ErrUnsupportedType, "%T", v)
	}

	t.args = append(t.args, v)
	return t.dialect.Placeholder(len(t.args)), nil
}

// validateRegex applies the translator-configured length cap from
// [filter.WithMaxRegexLength], falling back to
// [filter.DefaultMaxRegexLength] when the option was not set. It bounds
// pattern-compilation cost; whether the backend's engine can backtrack
// is a per-dialect concern documented by each package.
func (t *Translator) validateRegex(pattern string) error {
	maxLen := t.config.MaxRegexLength()
	if maxLen <= 0 {
		maxLen = filter.DefaultMaxRegexLength
	}
	return filter.ValidateRegex(pattern, maxLen)
}

// checkDepth verifies we haven't exceeded maximum nesting depth.
func (t *Translator) checkDepth() error {
	if t.depth >= t.config.MaxDepth() {
		return coreerrs.Wrapf(filter.ErrMaxDepthExceeded, "depth %d exceeds maximum %d", t.depth, t.config.MaxDepth())
	}
	return nil
}

// comparisonOperator maps a filter operator to its SQL spelling. All six
// are spelled identically across the supported dialects.
func comparisonOperator(op filter.Operator) (string, error) {
	switch op {
	case filter.OpEqual:
		return "=", nil
	case filter.OpNotEqual:
		return "!=", nil
	case filter.OpLT:
		return "<", nil
	case filter.OpLTE:
		return "<=", nil
	case filter.OpGT:
		return ">", nil
	case filter.OpGTE:
		return ">=", nil
	default:
		return "", coreerrs.Wrapf(filter.ErrUnsupportedOperation, "comparison operator %v", op)
	}
}

// isNullLiteral reports whether n is the CEL `null` literal.
func isNullLiteral(n filter.Node) bool {
	lit, ok := n.(*filter.LiteralNode)
	return ok && lit.Value == nil
}

// supportedValue reports whether a Go value can be bound as a query
// argument. The set is the intersection of what the CEL parser produces
// and what the SQL drivers accept.
func supportedValue(v any) bool {
	switch v.(type) {
	case bool, int64, uint64, float64, string, []byte, time.Time:
		return true
	default:
		return false
	}
}

// Ensure Translator implements filter.Visitor.
var _ filter.Visitor = (*Translator)(nil)
