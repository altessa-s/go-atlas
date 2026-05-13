// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewParser(t *testing.T) {
	t.Run("default options", func(t *testing.T) {
		p, err := NewParser()
		require.NoError(t, err)
		require.NotNil(t, p)
		require.NotNil(t, p.cache, "expected cache to be enabled by default")
	})

	t.Run("with custom cache size", func(t *testing.T) {
		p, err := NewParser(WithParserCacheSize(500))
		require.NoError(t, err)
		require.NotNil(t, p.cache, "expected cache to be enabled")
	})

	t.Run("with no cache", func(t *testing.T) {
		p, err := NewParser(WithParserNoCache())
		require.NoError(t, err)
		require.Nil(t, p.cache, "expected cache to be nil")
	})
}

func TestParser_Parse_EmptyExpression(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	tests := []struct {
		name string
		expr string
	}{
		{"empty", ""},
		{"whitespace", "   "},
		{"tabs", "\t\t"},
		{"newlines", "\n\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.Parse(t.Context(), tt.expr)
			require.ErrorIs(t, err, ErrEmptyExpression, "Parse(%q)", tt.expr)
		})
	}
}

func TestParser_Parse_Literals(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	tests := []struct {
		name     string
		expr     string
		wantKind NodeKind
		wantVal  any
	}{
		{"string", `"hello"`, NodeKindLiteral, "hello"},
		{"int", `42`, NodeKindLiteral, int64(42)},
		{"float", `3.14`, NodeKindLiteral, 3.14},
		{"bool true", `true`, NodeKindLiteral, true},
		{"bool false", `false`, NodeKindLiteral, false},
		{"null", `null`, NodeKindLiteral, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			require.NoError(t, err, "Parse(%q)", tt.expr)
			require.Equal(t, tt.wantKind, node.Kind(), "Parse(%q).Kind()", tt.expr)
			lit, ok := node.(*LiteralNode)
			require.True(t, ok, "Parse(%q) did not return LiteralNode", tt.expr)
			require.Equal(t, tt.wantVal, lit.Value, "Parse(%q).Value", tt.expr)
		})
	}
}

func TestParser_Parse_Identifiers(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	tests := []struct {
		name     string
		expr     string
		wantName string
	}{
		{"simple", `name`, "name"},
		{"nested", `address.city`, "address.city"},
		{"deeply nested", `user.profile.address.street`, "user.profile.address.street"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			require.NoError(t, err, "Parse(%q)", tt.expr)
			ident, ok := node.(*IdentNode)
			require.True(t, ok, "Parse(%q) did not return IdentNode, got %T", tt.expr, node)
			require.Equal(t, tt.wantName, ident.Name, "Parse(%q).Name", tt.expr)
		})
	}
}

func TestParser_Parse_ComparisonOperators(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	tests := []struct {
		name   string
		expr   string
		wantOp Operator
	}{
		{"equal", `name == "John"`, OpEqual},
		{"not equal", `name != "John"`, OpNotEqual},
		{"less than", `age < 18`, OpLT},
		{"less than or equal", `age <= 18`, OpLTE},
		{"greater than", `age > 18`, OpGT},
		{"greater than or equal", `age >= 18`, OpGTE},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			require.NoError(t, err, "Parse(%q)", tt.expr)
			binOp, ok := node.(*BinaryOpNode)
			require.True(t, ok, "Parse(%q) did not return BinaryOpNode, got %T", tt.expr, node)
			require.Equal(t, tt.wantOp, binOp.Op, "Parse(%q).Op", tt.expr)
		})
	}
}

func TestParser_Parse_LogicalOperators(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	tests := []struct {
		name   string
		expr   string
		wantOp Operator
	}{
		{"and", `name == "John" && age >= 18`, OpAnd},
		{"or", `status == "active" || status == "pending"`, OpOr},
		{"not", `!active`, OpNot},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			require.NoError(t, err, "Parse(%q)", tt.expr)
			switch tt.wantOp {
			case OpAnd, OpOr:
				binOp, ok := node.(*BinaryOpNode)
				require.True(t, ok, "Parse(%q) did not return BinaryOpNode, got %T", tt.expr, node)
				require.Equal(t, tt.wantOp, binOp.Op, "Parse(%q).Op", tt.expr)
			case OpNot:
				unaryOp, ok := node.(*UnaryOpNode)
				require.True(t, ok, "Parse(%q) did not return UnaryOpNode, got %T", tt.expr, node)
				require.Equal(t, tt.wantOp, unaryOp.Op, "Parse(%q).Op", tt.expr)
			}
		})
	}
}

func TestParser_Parse_MembershipOperator(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	node, err := p.Parse(t.Context(), `status in ["active", "pending"]`)
	require.NoError(t, err)
	binOp, ok := node.(*BinaryOpNode)
	require.True(t, ok, "Parse() did not return BinaryOpNode, got %T", node)
	require.Equal(t, OpIn, binOp.Op)

	ident, ok := binOp.Left.(*IdentNode)
	require.True(t, ok, "Left is not IdentNode, got %T", binOp.Left)
	require.Equal(t, "status", ident.Name)

	list, ok := binOp.Right.(*ListNode)
	require.True(t, ok, "Right is not ListNode, got %T", binOp.Right)
	require.Len(t, list.Elements, 2)
}

func TestParser_Parse_StringFunctions(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	tests := []struct {
		name   string
		expr   string
		wantOp Operator
	}{
		{"contains", `name.contains("oh")`, OpContains},
		{"startsWith", `name.startsWith("J")`, OpStartsWith},
		{"endsWith", `name.endsWith("n")`, OpEndsWith},
		{"matches", `name.matches("^[A-Z].*")`, OpMatches},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			require.NoError(t, err, "Parse(%q)", tt.expr)
			call, ok := node.(*CallNode)
			require.True(t, ok, "Parse(%q) did not return CallNode, got %T", tt.expr, node)
			require.Equal(t, tt.wantOp, call.Op, "Parse(%q).Op", tt.expr)
			require.NotNil(t, call.Target, "call.Target is nil")
			require.Len(t, call.Args, 1)
		})
	}
}

func TestParser_Parse_Substring(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	tests := []struct {
		name     string
		expr     string
		wantArgs int
	}{
		{"one-arg", `recipient.substring(7)`, 1},
		{"two-arg", `recipient.substring(0, 4)`, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			require.NoError(t, err, "Parse(%q)", tt.expr)
			call, ok := node.(*CallNode)
			require.True(t, ok, "Parse(%q) did not return CallNode, got %T", tt.expr, node)
			require.Equal(t, OpSubstring, call.Op, "Parse(%q).Op", tt.expr)
			require.NotNil(t, call.Target, "call.Target is nil")
			require.Len(t, call.Args, tt.wantArgs, "Parse(%q).Args", tt.expr)
		})
	}
}

func TestParser_Parse_SizeFunction(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	node, err := p.Parse(t.Context(), `tags.size()`)
	require.NoError(t, err)
	call, ok := node.(*CallNode)
	require.True(t, ok, "Parse() did not return CallNode, got %T", node)
	require.Equal(t, OpSize, call.Op)
	require.NotNil(t, call.Target, "call.Target is nil")
}

func TestParser_Parse_HasMacro(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	node, err := p.Parse(t.Context(), `has(user.email)`)
	require.NoError(t, err)
	call, ok := node.(*CallNode)
	require.True(t, ok, "Parse() did not return CallNode, got %T", node)
	require.Equal(t, OpHas, call.Op)
}

func TestParser_Parse_Timestamp(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	node, err := p.Parse(t.Context(), `timestamp("2024-01-15T10:30:00Z")`)
	require.NoError(t, err)
	lit, ok := node.(*LiteralNode)
	require.True(t, ok, "Parse() did not return LiteralNode, got %T", node)
	ts, ok := lit.Value.(time.Time)
	require.True(t, ok, "Value is not time.Time, got %T", lit.Value)
	expected := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	require.True(t, ts.Equal(expected), "timestamp = %v, want %v", ts, expected)
}

func TestParser_Parse_ComplexExpressions(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	tests := []struct {
		name string
		expr string
	}{
		{"compound and", `name == "John" && age >= 18 && active == true`},
		{"compound or", `status == "active" || status == "pending" || status == "new"`},
		{"mixed", `(name == "John" || name == "Jane") && age >= 18`},
		{"nested field comparison", `address.city == "NYC" && address.zip == "10001"`},
		{"in with and", `status in ["active", "pending"] && age >= 18`},
		{"string func with and", `name.startsWith("J") && active == true`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			require.NoError(t, err, "Parse(%q)", tt.expr)
			require.NotNil(t, node, "Parse() returned nil node")
		})
	}
}

func TestParser_Parse_InvalidExpressions(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	tests := []struct {
		name string
		expr string
	}{
		{"unclosed string", `name == "John`},
		{"unclosed paren", `(name == "John"`},
		{"invalid operator", `name === "John"`},
		{"missing operand", `name ==`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.Parse(t.Context(), tt.expr)
			require.Error(t, err, "Parse(%q) expected error", tt.expr)
		})
	}
}

func TestParser_MustParse_Panic(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	require.Panics(t, func() {
		p.MustParse(`invalid ===`)
	}, "MustParse() did not panic on invalid expression")
}

func TestParser_MustParse_Success(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	node := p.MustParse(`name == "John"`)
	require.NotNil(t, node, "MustParse() returned nil")
}

func TestParser_Cache(t *testing.T) {
	p, _ := NewParser()

	expr := `name == "John"`

	node1, err := p.Parse(t.Context(), expr)
	require.NoError(t, err, "first Parse()")

	node2, err := p.Parse(t.Context(), expr)
	require.NoError(t, err, "second Parse()")

	require.Same(t, node1, node2, "cache did not return same node instance")
}

func TestNodeKind_String(t *testing.T) {
	tests := []struct {
		kind NodeKind
		want string
	}{
		{NodeKindLiteral, "NodeKindLiteral"},
		{NodeKindIdent, "NodeKindIdent"},
		{NodeKindBinaryOp, "NodeKindBinaryOp"},
		{NodeKindUnaryOp, "NodeKindUnaryOp"},
		{NodeKindCall, "NodeKindCall"},
		{NodeKindList, "NodeKindList"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			require.Equal(t, tt.want, tt.kind.String())
		})
	}

	// Test unknown kind returns formatted string
	unknown := NodeKind(99)
	require.True(t, strings.HasPrefix(unknown.String(), "NodeKind("), "Unknown NodeKind.String() = %q", unknown.String())
}

func TestOperator_String(t *testing.T) {
	tests := []struct {
		op   Operator
		want string
	}{
		{OpEqual, "OpEqual"},
		{OpNotEqual, "OpNotEqual"},
		{OpLT, "OpLT"},
		{OpLTE, "OpLTE"},
		{OpGT, "OpGT"},
		{OpGTE, "OpGTE"},
		{OpAnd, "OpAnd"},
		{OpOr, "OpOr"},
		{OpNot, "OpNot"},
		{OpIn, "OpIn"},
		{OpContains, "OpContains"},
		{OpStartsWith, "OpStartsWith"},
		{OpEndsWith, "OpEndsWith"},
		{OpMatches, "OpMatches"},
		{OpSize, "OpSize"},
		{OpHas, "OpHas"},
		{OpExists, "OpExists"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			require.Equal(t, tt.want, tt.op.String())
		})
	}

	// Test unknown operator returns formatted string
	unknown := Operator(99)
	require.True(t, strings.HasPrefix(unknown.String(), "Operator("), "Unknown Operator.String() = %q", unknown.String())
}

func TestOperator_IsComparison(t *testing.T) {
	comparisons := []Operator{OpEqual, OpNotEqual, OpLT, OpLTE, OpGT, OpGTE}
	for _, op := range comparisons {
		require.True(t, op.IsComparison(), "%v.IsComparison()", op)
	}

	nonComparisons := []Operator{OpAnd, OpOr, OpNot, OpIn, OpContains}
	for _, op := range nonComparisons {
		require.False(t, op.IsComparison(), "%v.IsComparison()", op)
	}
}

func TestOperator_IsLogical(t *testing.T) {
	logical := []Operator{OpAnd, OpOr, OpNot}
	for _, op := range logical {
		require.True(t, op.IsLogical(), "%v.IsLogical()", op)
	}

	nonLogical := []Operator{OpEqual, OpIn, OpContains}
	for _, op := range nonLogical {
		require.False(t, op.IsLogical(), "%v.IsLogical()", op)
	}
}

func TestOperator_IsStringOp(t *testing.T) {
	stringOps := []Operator{OpContains, OpStartsWith, OpEndsWith, OpMatches}
	for _, op := range stringOps {
		require.True(t, op.IsStringOp(), "%v.IsStringOp()", op)
	}

	nonStringOps := []Operator{OpEqual, OpAnd, OpIn, OpSize, OpSubstring}
	for _, op := range nonStringOps {
		require.False(t, op.IsStringOp(), "%v.IsStringOp()", op)
	}
}

func TestWalk(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())
	node, _ := p.Parse(t.Context(), `name == "John" && age >= 18`)

	count := 0
	Walk(node, func(n Node) bool {
		count++
		return true
	})

	require.NotZero(t, count, "Walk did not visit any nodes")
}

func TestAllNodes(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())
	node, _ := p.Parse(t.Context(), `name == "John"`)

	count := 0
	for range AllNodes(node) {
		count++
	}

	require.Equal(t, CountNodes(node), count)
}

func TestDepth(t *testing.T) {
	tests := []struct {
		expr     string
		minDepth int
	}{
		{`"hello"`, 1},
		{`name == "John"`, 2},
		{`name == "John" && age >= 18`, 3},
	}

	p, _ := NewParser(WithParserNoCache())
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			node, _ := p.Parse(t.Context(), tt.expr)
			d := Depth(node)
			require.GreaterOrEqual(t, d, tt.minDepth, "Depth(%q)", tt.expr)
		})
	}

	// Test nil
	require.Equal(t, 0, Depth(nil), "Depth(nil) should be 0")
}

func TestParser_Parse_ExpressionTooLong(t *testing.T) {
	p, _ := NewParser(WithParserNoCache(), WithMaxExpressionLength(50))

	t.Run("expression within limit", func(t *testing.T) {
		_, err := p.Parse(t.Context(), `name == "John"`)
		require.NoError(t, err)
	})

	t.Run("expression exceeding limit", func(t *testing.T) {
		long := `name == "` + strings.Repeat("a", 50) + `"`
		_, err := p.Parse(t.Context(), long)
		require.ErrorIs(t, err, ErrExpressionTooLong)
	})

	t.Run("default limit allows reasonable expressions", func(t *testing.T) {
		pDefault, _ := NewParser(WithParserNoCache())
		_, err := pDefault.Parse(t.Context(), `name == "John" && age >= 18`)
		require.NoError(t, err)
	})
}

func TestValidateRegex(t *testing.T) {
	t.Run("valid short regex", func(t *testing.T) {
		require.NoError(t, ValidateRegex("^hello.*", 1024))
	})

	t.Run("regex exceeding length", func(t *testing.T) {
		long := strings.Repeat("a", 1025)
		require.ErrorIs(t, ValidateRegex(long, 1024), ErrInvalidRegex)
	})

	t.Run("invalid regex pattern", func(t *testing.T) {
		require.ErrorIs(t, ValidateRegex("[invalid", 1024), ErrInvalidRegex)
	})
}

func TestParser_Parse_DefaultExpressionLength(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	// Build an expression that exceeds DefaultMaxExpressionLength (4096)
	long := `name == "` + strings.Repeat("x", DefaultMaxExpressionLength) + `"`
	_, err := p.Parse(t.Context(), long)
	require.ErrorIs(t, err, ErrExpressionTooLong)
}

func TestNode_Children(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	t.Run("LiteralNode has no children", func(t *testing.T) {
		node, _ := p.Parse(t.Context(), `"hello"`)
		count := 0
		for range node.Children() {
			count++
		}
		require.Equal(t, 0, count, "LiteralNode children")
	})

	t.Run("BinaryOpNode has 2 children", func(t *testing.T) {
		node, _ := p.Parse(t.Context(), `name == "John"`)
		count := 0
		for range node.Children() {
			count++
		}
		require.Equal(t, 2, count, "BinaryOpNode children")
	})

	t.Run("ListNode has children for each element", func(t *testing.T) {
		node, _ := p.Parse(t.Context(), `status in ["a", "b", "c"]`)
		binOp := node.(*BinaryOpNode)
		list := binOp.Right.(*ListNode)
		count := 0
		for range list.Children() {
			count++
		}
		require.Equal(t, 3, count, "ListNode children")
	})
}

func TestParser_Parse_CustomFunction_CompareField(t *testing.T) {
	p, err := NewParser(
		WithParserNoCache(),
		WithCustomFunctions(map[string]CustomFunction{
			"createdAfter": CompareField("createdAt", OpGT),
		}),
	)
	require.NoError(t, err)

	node, err := p.Parse(t.Context(), `createdAfter("2024-01-01")`)
	require.NoError(t, err)

	binOp, ok := node.(*BinaryOpNode)
	require.True(t, ok, "expected BinaryOpNode, got %T", node)
	require.Equal(t, OpGT, binOp.Op)

	ident, ok := binOp.Left.(*IdentNode)
	require.True(t, ok, "Left is not IdentNode, got %T", binOp.Left)
	require.Equal(t, "createdAt", ident.Name)

	lit, ok := binOp.Right.(*LiteralNode)
	require.True(t, ok, "Right is not LiteralNode, got %T", binOp.Right)
	require.Equal(t, "2024-01-01", lit.Value)
}

func TestParser_Parse_CustomFunction_HandlerError(t *testing.T) {
	p, err := NewParser(
		WithParserNoCache(),
		WithCustomFunctions(map[string]CustomFunction{
			"alwaysFails": func([]Node) (Node, error) {
				return nil, errors.New("nope")
			},
		}),
	)
	require.NoError(t, err)

	_, err = p.Parse(t.Context(), `alwaysFails(1)`)
	require.ErrorIs(t, err, ErrInvalidExpression)
	require.Contains(t, err.Error(), "alwaysFails")
}

func TestParser_Parse_CustomFunction_NilNode(t *testing.T) {
	p, err := NewParser(
		WithParserNoCache(),
		WithCustomFunctions(map[string]CustomFunction{
			"returnsNil": func([]Node) (Node, error) { return nil, nil },
		}),
	)
	require.NoError(t, err)

	_, err = p.Parse(t.Context(), `returnsNil(1)`)
	require.ErrorIs(t, err, ErrInvalidExpression)
	require.Contains(t, err.Error(), "nil node")
}

func TestParser_Parse_CustomFunction_Composition(t *testing.T) {
	p, err := NewParser(
		WithParserNoCache(),
		WithCustomFunctions(map[string]CustomFunction{
			"createdAfter": CompareField("createdAt", OpGT),
		}),
	)
	require.NoError(t, err)

	node, err := p.Parse(t.Context(), `createdAfter("2024-01-01") && status == "ok"`)
	require.NoError(t, err)

	andNode, ok := node.(*BinaryOpNode)
	require.True(t, ok, "expected BinaryOpNode, got %T", node)
	require.Equal(t, OpAnd, andNode.Op)

	left, ok := andNode.Left.(*BinaryOpNode)
	require.True(t, ok, "Left is not BinaryOpNode, got %T", andNode.Left)
	require.Equal(t, OpGT, left.Op)
	leftIdent, ok := left.Left.(*IdentNode)
	require.True(t, ok)
	require.Equal(t, "createdAt", leftIdent.Name)
}

func TestParser_Parse_CustomFunction_BetweenHandler(t *testing.T) {
	between := func(args []Node) (Node, error) {
		if len(args) != 3 {
			return nil, errors.New("between requires 3 args")
		}
		field, ok := args[0].(*IdentNode)
		if !ok {
			return nil, errors.New("first arg must be a field identifier")
		}
		return &BinaryOpNode{
			Op:    OpAnd,
			Left:  &BinaryOpNode{Op: OpGTE, Left: field, Right: args[1]},
			Right: &BinaryOpNode{Op: OpLTE, Left: field, Right: args[2]},
		}, nil
	}

	p, err := NewParser(
		WithParserNoCache(),
		WithCustomFunctions(map[string]CustomFunction{"between": between}),
	)
	require.NoError(t, err)

	node, err := p.Parse(t.Context(), `between(age, 18, 65)`)
	require.NoError(t, err)

	andNode, ok := node.(*BinaryOpNode)
	require.True(t, ok, "expected BinaryOpNode, got %T", node)
	require.Equal(t, OpAnd, andNode.Op)
	require.Equal(t, OpGTE, andNode.Left.(*BinaryOpNode).Op)
	require.Equal(t, OpLTE, andNode.Right.(*BinaryOpNode).Op)
}

func TestParser_Parse_CustomFunction_UnknownFallsThrough(t *testing.T) {
	p, err := NewParser(
		WithParserNoCache(),
		WithCustomFunctions(map[string]CustomFunction{
			"createdAfter": CompareField("createdAt", OpGT),
		}),
	)
	require.NoError(t, err)

	_, err = p.Parse(t.Context(), `unknownFunc(1)`)
	require.ErrorIs(t, err, ErrUnsupportedOperation)
}

func TestNewParser_CustomFunctions_ReservedName(t *testing.T) {
	_, err := NewParser(WithCustomFunctions(map[string]CustomFunction{
		"contains": func([]Node) (Node, error) { return nil, nil },
	}))
	require.ErrorIs(t, err, ErrInvalidExpression)
	require.Contains(t, err.Error(), "contains")
}

func TestNewParser_CustomFunctions_NilHandler(t *testing.T) {
	_, err := NewParser(WithCustomFunctions(map[string]CustomFunction{
		"createdAfter": nil,
	}))
	require.ErrorIs(t, err, ErrInvalidExpression)
	require.Contains(t, err.Error(), "nil handler")
}

func TestCompareField_NonComparisonOperator(t *testing.T) {
	h := CompareField("createdAt", OpAnd)
	_, err := h([]Node{&LiteralNode{Value: "x"}})
	require.ErrorIs(t, err, ErrInvalidExpression)
	require.Contains(t, err.Error(), "not a comparison")
}

func TestCompareField_WrongArity(t *testing.T) {
	h := CompareField("createdAt", OpGT)

	_, err := h(nil)
	require.ErrorIs(t, err, ErrInvalidExpression)

	_, err = h([]Node{&LiteralNode{Value: 1}, &LiteralNode{Value: 2}})
	require.ErrorIs(t, err, ErrInvalidExpression)
}

func TestParser_Parse_AllowedFunctions(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr error
	}{
		{"allowed built-in", `name.contains("a")`, nil},
		{"blocked built-in", `name.matches("a")`, ErrFunctionNotAllowed},
		{"allowed custom", `createdAfter("2024-01-01")`, nil},
		{"unregistered custom (blocked by allowlist first)", `unknownFunc(1)`, ErrFunctionNotAllowed},
		{"operator: equality", `name == "John"`, nil},
		{"operator: logical and", `name == "John" && active`, nil},
		{"operator: logical or", `a == 1 || b == 2`, nil},
		{"operator: in", `status in ["a", "b"]`, nil},
		{"operator: unary not", `!active`, nil},
		{"macro: has", `has(user.email)`, nil},
		{"comparison chain", `age >= 18 && age < 65`, nil},
	}

	p, err := NewParser(
		WithParserNoCache(),
		WithAllowedFunctions("contains", "createdAfter"),
		WithCustomFunctions(map[string]CustomFunction{
			"createdAfter": CompareField("createdAt", OpGT),
		}),
	)
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.Parse(t.Context(), tt.expr)
			if tt.wantErr == nil {
				require.NoError(t, err, "Parse(%q)", tt.expr)
				return
			}
			require.ErrorIs(t, err, tt.wantErr, "Parse(%q)", tt.expr)
		})
	}
}

func TestParser_Parse_AllowedFunctions_AllowedButUnregistered(t *testing.T) {
	p, err := NewParser(
		WithParserNoCache(),
		WithAllowedFunctions("createdAfter"),
		// no custom registration for createdAfter
	)
	require.NoError(t, err)

	_, err = p.Parse(t.Context(), `createdAfter("2024-01-01")`)
	require.ErrorIs(t, err, ErrUnsupportedOperation,
		"allowlist passes but no handler exists, must surface as ErrUnsupportedOperation")
}

func TestParser_Parse_AllowedFunctions_NotConfiguredAllowsAll(t *testing.T) {
	p, err := NewParser(WithParserNoCache())
	require.NoError(t, err)

	_, err = p.Parse(t.Context(), `name.matches("^J.*")`)
	require.NoError(t, err, "without WithAllowedFunctions every function is allowed")
}

// withCleanGlobalRegistry resets the package-level registry before and
// after the test so global-registry tests don't leak into one another.
// These tests must not run in parallel with each other.
func withCleanGlobalRegistry(t *testing.T) {
	t.Helper()
	ResetGlobalCustomFunctions()
	t.Cleanup(ResetGlobalCustomFunctions)
}

func TestRegisterFunctions_ParserPicksUpGlobal(t *testing.T) {
	withCleanGlobalRegistry(t)

	require.NoError(t, RegisterFunctions(map[string]CustomFunction{
		"createdAfter": CompareField("createdAt", OpGT),
	}))

	p, err := NewParser(WithParserNoCache())
	require.NoError(t, err)

	node, err := p.Parse(t.Context(), `createdAfter("2024-01-01")`)
	require.NoError(t, err)

	bin, ok := node.(*BinaryOpNode)
	require.True(t, ok, "expected BinaryOpNode, got %T", node)
	require.Equal(t, OpGT, bin.Op)
	require.Equal(t, "createdAt", bin.Left.(*IdentNode).Name)
}

func TestRegisterFunctions_PerParserOverride(t *testing.T) {
	withCleanGlobalRegistry(t)

	require.NoError(t, RegisterFunctions(map[string]CustomFunction{
		"shortcut": CompareField("createdAt", OpGT),
	}))

	override := func([]Node) (Node, error) {
		return &LiteralNode{Value: "override"}, nil
	}
	p, err := NewParser(
		WithParserNoCache(),
		WithCustomFunctions(map[string]CustomFunction{"shortcut": override}),
	)
	require.NoError(t, err)

	node, err := p.Parse(t.Context(), `shortcut(1)`)
	require.NoError(t, err)
	lit, ok := node.(*LiteralNode)
	require.True(t, ok, "expected LiteralNode, got %T", node)
	require.Equal(t, "override", lit.Value)
}

func TestRegisterFunctions_WithoutGlobal(t *testing.T) {
	withCleanGlobalRegistry(t)

	require.NoError(t, RegisterFunctions(map[string]CustomFunction{
		"createdAfter": CompareField("createdAt", OpGT),
	}))

	p, err := NewParser(WithParserNoCache(), WithoutGlobalCustomFunctions())
	require.NoError(t, err)

	_, err = p.Parse(t.Context(), `createdAfter("2024-01-01")`)
	require.ErrorIs(t, err, ErrUnsupportedOperation,
		"opt-out parser must not see globally registered functions")
}

func TestRegisterFunctions_DuplicateName(t *testing.T) {
	withCleanGlobalRegistry(t)

	require.NoError(t, RegisterFunctions(map[string]CustomFunction{
		"createdAfter": CompareField("createdAt", OpGT),
	}))

	err := RegisterFunctions(map[string]CustomFunction{
		"createdAfter": CompareField("created_at", OpGT),
	})
	require.ErrorIs(t, err, ErrInvalidExpression)
	require.Contains(t, err.Error(), "already registered")
}

func TestRegisterFunctions_Validation(t *testing.T) {
	withCleanGlobalRegistry(t)

	t.Run("reserved name", func(t *testing.T) {
		err := RegisterFunctions(map[string]CustomFunction{
			"contains": func([]Node) (Node, error) { return nil, nil },
		})
		require.ErrorIs(t, err, ErrInvalidExpression)
	})

	t.Run("nil handler", func(t *testing.T) {
		err := RegisterFunctions(map[string]CustomFunction{
			"createdAfter": nil,
		})
		require.ErrorIs(t, err, ErrInvalidExpression)
	})
}

func TestGlobalCustomFunctions_Snapshot(t *testing.T) {
	withCleanGlobalRegistry(t)

	require.Empty(t, GlobalCustomFunctions())

	require.NoError(t, RegisterFunctions(map[string]CustomFunction{
		"createdAfter": CompareField("createdAt", OpGT),
	}))

	snap := GlobalCustomFunctions()
	require.Len(t, snap, 1)
	require.NotNil(t, snap["createdAfter"])

	// Mutating the snapshot must not affect the registry.
	delete(snap, "createdAfter")
	require.Len(t, GlobalCustomFunctions(), 1)
}

func TestRegisterFunctions_CombinesWithPerParser(t *testing.T) {
	withCleanGlobalRegistry(t)

	require.NoError(t, RegisterFunctions(map[string]CustomFunction{
		"createdAfter": CompareField("createdAt", OpGT),
	}))

	p, err := NewParser(
		WithParserNoCache(),
		WithCustomFunctions(map[string]CustomFunction{
			"updatedAfter": CompareField("updatedAt", OpGT),
		}),
	)
	require.NoError(t, err)

	for _, expr := range []string{`createdAfter("2024-01-01")`, `updatedAfter("2024-01-02")`} {
		t.Run(expr, func(t *testing.T) {
			_, err := p.Parse(t.Context(), expr)
			require.NoError(t, err, "Parse(%q)", expr)
		})
	}
}

func TestIsUserFunction(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"_==_", false},
		{"_!=_", false},
		{"_<_", false},
		{"_<=_", false},
		{"_>_", false},
		{"_>=_", false},
		{"_&&_", false},
		{"_||_", false},
		{"!_", false},
		{"@in", false},
		{"has", false},
		{"contains", true},
		{"startsWith", true},
		{"endsWith", true},
		{"matches", true},
		{"size", true},
		{"timestamp", true},
		{"createdAfter", true},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isUserFunction(tt.name))
		})
	}
}
