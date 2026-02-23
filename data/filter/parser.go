// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/operators"
	"github.com/google/cel-go/common/types"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/data/cache/lru"

	"google.golang.org/protobuf/types/known/timestamppb"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

// DefaultParserCacheSize is the maximum number of parsed AST nodes to cache.
const DefaultParserCacheSize = 1000

// parserConfig holds parser configuration.
type parserConfig struct {
	cacheSize int
	noCache   bool
}

// defaultParserConfig returns default parser configuration.
func defaultParserConfig() *parserConfig {
	return &parserConfig{
		cacheSize: DefaultParserCacheSize,
		noCache:   false,
	}
}

// ParserOption configures the Parser.
type ParserOption func(*parserConfig)

// WithParserCacheSize sets the LRU cache size for parsed expressions.
func WithParserCacheSize(size int) ParserOption {
	return func(c *parserConfig) {
		if size > 0 {
			c.cacheSize = size
		}
	}
}

// WithParserNoCache disables caching of parsed expressions.
func WithParserNoCache() ParserOption {
	return func(c *parserConfig) {
		c.noCache = true
	}
}

// Parser parses CEL expressions into filter AST nodes.
type Parser struct {
	env   *cel.Env
	cache lru.Cacher[string, Node]
}

// getCELEnvironment returns the shared CEL environment, initialized on first call.
var getCELEnvironment = sync.OnceValues(func() (*cel.Env, error) {
	return cel.NewEnv(
		cel.EnableMacroCallTracking(),
	)
})

// NewParser creates a new CEL expression parser.
func NewParser(opts ...ParserOption) (*Parser, error) {
	cfg := defaultParserConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	env, err := getCELEnvironment()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParseFailed, err)
	}

	p := &Parser{env: env}

	if !cfg.noCache {
		cache, err := lru.NewCache[string, Node](cfg.cacheSize)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "create parser cache")
		}
		p.cache = cache
	}

	return p, nil
}

// Parse parses a CEL expression and returns the corresponding AST node.
func (p *Parser) Parse(expression string) (Node, error) {
	if corestrings.IsEmpty(expression) {
		return nil, ErrEmptyExpression
	}

	if p.cache != nil {
		return p.cache.GetOrCompute(context.Background(), expression, func(ctx context.Context) (Node, error) {
			return p.parseInternal(expression)
		})
	}

	return p.parseInternal(expression)
}

// MustParse parses a CEL expression and panics if parsing fails.
// Use this for compile-time expressions to catch errors at startup.
func (p *Parser) MustParse(expression string) Node {
	return panics.MustResult(p.Parse(expression))
}

// parseInternal performs the actual parsing without caching.
func (p *Parser) parseInternal(expression string) (Node, error) {
	ast, issues := p.env.Parse(expression)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("%w: %v", ErrParseFailed, issues.Err())
	}

	parsedExpr, err := cel.AstToParsedExpr(ast)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParseFailed, err)
	}
	return p.convertExpr(parsedExpr.GetExpr())
}

// convertExpr converts a CEL expression to our AST node.
func (p *Parser) convertExpr(expr *exprpb.Expr) (Node, error) {
	if expr == nil {
		return nil, fmt.Errorf("%w: nil expression", ErrInvalidExpression)
	}

	switch e := expr.ExprKind.(type) {
	case *exprpb.Expr_ConstExpr:
		return p.convertConst(e.ConstExpr)
	case *exprpb.Expr_IdentExpr:
		return &IdentNode{Name: e.IdentExpr.Name}, nil
	case *exprpb.Expr_SelectExpr:
		return p.convertSelect(e.SelectExpr)
	case *exprpb.Expr_CallExpr:
		return p.convertCall(e.CallExpr)
	case *exprpb.Expr_ListExpr:
		return p.convertList(e.ListExpr)
	default:
		return nil, fmt.Errorf("%w: unsupported expression type %T", ErrInvalidExpression, e)
	}
}

// convertConst converts a CEL constant to a LiteralNode.
func (p *Parser) convertConst(c *exprpb.Constant) (Node, error) {
	switch k := c.ConstantKind.(type) {
	case *exprpb.Constant_BoolValue:
		return &LiteralNode{Value: k.BoolValue}, nil
	case *exprpb.Constant_Int64Value:
		return &LiteralNode{Value: k.Int64Value}, nil
	case *exprpb.Constant_Uint64Value:
		return &LiteralNode{Value: k.Uint64Value}, nil
	case *exprpb.Constant_DoubleValue:
		return &LiteralNode{Value: k.DoubleValue}, nil
	case *exprpb.Constant_StringValue:
		return &LiteralNode{Value: k.StringValue}, nil
	case *exprpb.Constant_BytesValue:
		return &LiteralNode{Value: k.BytesValue}, nil
	case *exprpb.Constant_NullValue:
		return &LiteralNode{Value: nil}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported constant type %T", ErrUnsupportedType, k)
	}
}

// convertSelect converts a CEL select expression (field access) to an IdentNode.
// If TestOnly is true, this represents a has() macro call.
func (p *Parser) convertSelect(s *exprpb.Expr_Select) (Node, error) {
	path, err := p.buildFieldPath(s)
	if err != nil {
		return nil, err
	}

	if s.TestOnly {
		return &CallNode{
			Op:     OpHas,
			Target: &IdentNode{Name: path},
		}, nil
	}

	return &IdentNode{Name: path}, nil
}

// buildFieldPath recursively builds a dotted field path from a select expression.
func (p *Parser) buildFieldPath(s *exprpb.Expr_Select) (string, error) {
	parts := make([]string, 0, 4) // Pre-allocate for typical depth

	operand := s.Operand
	for operand != nil {
		switch e := operand.ExprKind.(type) {
		case *exprpb.Expr_IdentExpr:
			parts = append([]string{e.IdentExpr.Name}, parts...)
			operand = nil
		case *exprpb.Expr_SelectExpr:
			parts = append([]string{e.SelectExpr.Field}, parts...)
			operand = e.SelectExpr.Operand
		default:
			return "", fmt.Errorf("%w: cannot build field path from %T", ErrInvalidExpression, e)
		}
	}

	parts = append(parts, s.Field)
	return corestrings.Join(parts, corestrings.JoinOptions{Separator: "."}), nil
}

// convertCall converts a CEL call expression to the appropriate node type.
func (p *Parser) convertCall(c *exprpb.Expr_Call) (Node, error) {
	switch c.Function {
	// Comparison operators
	case operators.Equals:
		return p.convertBinaryOp(OpEqual, c.Args)
	case operators.NotEquals:
		return p.convertBinaryOp(OpNotEqual, c.Args)
	case operators.Less:
		return p.convertBinaryOp(OpLT, c.Args)
	case operators.LessEquals:
		return p.convertBinaryOp(OpLTE, c.Args)
	case operators.Greater:
		return p.convertBinaryOp(OpGT, c.Args)
	case operators.GreaterEquals:
		return p.convertBinaryOp(OpGTE, c.Args)

	// Logical operators
	case operators.LogicalAnd:
		return p.convertBinaryOp(OpAnd, c.Args)
	case operators.LogicalOr:
		return p.convertBinaryOp(OpOr, c.Args)
	case operators.LogicalNot:
		return p.convertUnaryOp(OpNot, c.Args)

	// Membership
	case operators.In:
		return p.convertBinaryOp(OpIn, c.Args)

	// String functions
	case "contains":
		return p.convertMethodCall(OpContains, c.Target, c.Args)
	case "startsWith":
		return p.convertMethodCall(OpStartsWith, c.Target, c.Args)
	case "endsWith":
		return p.convertMethodCall(OpEndsWith, c.Target, c.Args)
	case "matches":
		return p.convertMethodCall(OpMatches, c.Target, c.Args)

	// Size function
	case "size":
		return p.convertMethodCall(OpSize, c.Target, c.Args)

	// Has macro
	case operators.Has:
		if len(c.Args) != 1 {
			return nil, fmt.Errorf("%w: has() requires exactly one argument", ErrInvalidExpression)
		}
		arg, err := p.convertExpr(c.Args[0])
		if err != nil {
			return nil, err
		}
		return &CallNode{Op: OpHas, Target: arg}, nil

	// Timestamp function
	case "timestamp":
		return p.convertTimestamp(c.Args)

	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedOperation, c.Function)
	}
}

// convertTimestamp converts a timestamp() call to a LiteralNode with time.Time.
func (p *Parser) convertTimestamp(args []*exprpb.Expr) (Node, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: timestamp() requires exactly one argument", ErrInvalidExpression)
	}

	arg, err := p.convertExpr(args[0])
	if err != nil {
		return nil, err
	}

	lit, ok := arg.(*LiteralNode)
	if !ok {
		return nil, fmt.Errorf("%w: timestamp() requires a string argument", ErrInvalidExpression)
	}

	s, ok := lit.Value.(string)
	if !ok {
		return nil, fmt.Errorf("%w: timestamp() requires a string argument", ErrInvalidExpression)
	}

	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid timestamp format: %v", ErrInvalidExpression, err)
	}

	return &LiteralNode{Value: t}, nil
}

// convertBinaryOp creates a BinaryOpNode from arguments.
func (p *Parser) convertBinaryOp(op Operator, args []*exprpb.Expr) (Node, error) {
	const binaryArgCount = 2
	if len(args) != binaryArgCount {
		return nil, fmt.Errorf("%w: binary operator requires 2 arguments, got %d", ErrInvalidExpression, len(args))
	}

	left, err := p.convertExpr(args[0])
	if err != nil {
		return nil, err
	}

	right, err := p.convertExpr(args[1])
	if err != nil {
		return nil, err
	}

	return &BinaryOpNode{Op: op, Left: left, Right: right}, nil
}

// convertUnaryOp creates a UnaryOpNode from arguments.
func (p *Parser) convertUnaryOp(op Operator, args []*exprpb.Expr) (Node, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: unary operator requires 1 argument, got %d", ErrInvalidExpression, len(args))
	}

	operand, err := p.convertExpr(args[0])
	if err != nil {
		return nil, err
	}

	return &UnaryOpNode{Op: op, Operand: operand}, nil
}

// convertMethodCall creates a CallNode for method-style calls.
func (p *Parser) convertMethodCall(op Operator, target *exprpb.Expr, args []*exprpb.Expr) (Node, error) {
	var targetNode Node
	if target != nil {
		var err error
		targetNode, err = p.convertExpr(target)
		if err != nil {
			return nil, err
		}
	}

	argNodes := make([]Node, 0, len(args))
	for _, arg := range args {
		node, err := p.convertExpr(arg)
		if err != nil {
			return nil, err
		}
		argNodes = append(argNodes, node)
	}

	return &CallNode{Op: op, Target: targetNode, Args: argNodes}, nil
}

// convertList converts a CEL list expression to a ListNode.
func (p *Parser) convertList(l *exprpb.Expr_CreateList) (Node, error) {
	elements := make([]Node, 0, len(l.Elements))
	for _, elem := range l.Elements {
		node, err := p.convertExpr(elem)
		if err != nil {
			return nil, err
		}
		elements = append(elements, node)
	}
	return &ListNode{Elements: elements}, nil
}

// ConvertCELValue converts CEL runtime values to Go values.
func ConvertCELValue(val any) any {
	switch v := val.(type) {
	case types.Bool:
		return bool(v)
	case types.Int:
		return int64(v)
	case types.Uint:
		return uint64(v)
	case types.Double:
		return float64(v)
	case types.String:
		return string(v)
	case types.Bytes:
		return []byte(v)
	case types.Null:
		return nil
	case *timestamppb.Timestamp:
		return v.AsTime()
	default:
		return val
	}
}
