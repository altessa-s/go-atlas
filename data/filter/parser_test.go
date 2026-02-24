// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewParser(t *testing.T) {
	t.Run("default options", func(t *testing.T) {
		p, err := NewParser()
		if err != nil {
			t.Fatalf("NewParser() error = %v", err)
		}
		if p == nil {
			t.Fatal("NewParser() returned nil")
		}
		if p.cache == nil {
			t.Error("expected cache to be enabled by default")
		}
	})

	t.Run("with custom cache size", func(t *testing.T) {
		p, err := NewParser(WithParserCacheSize(500))
		if err != nil {
			t.Fatalf("NewParser() error = %v", err)
		}
		if p.cache == nil {
			t.Error("expected cache to be enabled")
		}
	})

	t.Run("with no cache", func(t *testing.T) {
		p, err := NewParser(WithParserNoCache())
		if err != nil {
			t.Fatalf("NewParser() error = %v", err)
		}
		if p.cache != nil {
			t.Error("expected cache to be nil")
		}
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
			_, err := p.Parse(context.Background(), tt.expr)
			if !errors.Is(err, ErrEmptyExpression) {
				t.Errorf("Parse(%q) error = %v, want %v", tt.expr, err, ErrEmptyExpression)
			}
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
			node, err := p.Parse(context.Background(), tt.expr)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tt.expr, err)
			}
			if node.Kind() != tt.wantKind {
				t.Errorf("Parse(%q).Kind() = %v, want %v", tt.expr, node.Kind(), tt.wantKind)
			}
			lit, ok := node.(*LiteralNode)
			if !ok {
				t.Fatalf("Parse(%q) did not return LiteralNode", tt.expr)
			}
			if lit.Value != tt.wantVal {
				t.Errorf("Parse(%q).Value = %v, want %v", tt.expr, lit.Value, tt.wantVal)
			}
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
			node, err := p.Parse(context.Background(), tt.expr)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tt.expr, err)
			}
			ident, ok := node.(*IdentNode)
			if !ok {
				t.Fatalf("Parse(%q) did not return IdentNode, got %T", tt.expr, node)
			}
			if ident.Name != tt.wantName {
				t.Errorf("Parse(%q).Name = %q, want %q", tt.expr, ident.Name, tt.wantName)
			}
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
			node, err := p.Parse(context.Background(), tt.expr)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tt.expr, err)
			}
			binOp, ok := node.(*BinaryOpNode)
			if !ok {
				t.Fatalf("Parse(%q) did not return BinaryOpNode, got %T", tt.expr, node)
			}
			if binOp.Op != tt.wantOp {
				t.Errorf("Parse(%q).Op = %v, want %v", tt.expr, binOp.Op, tt.wantOp)
			}
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
			node, err := p.Parse(context.Background(), tt.expr)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tt.expr, err)
			}
			switch tt.wantOp {
			case OpAnd, OpOr:
				binOp, ok := node.(*BinaryOpNode)
				if !ok {
					t.Fatalf("Parse(%q) did not return BinaryOpNode, got %T", tt.expr, node)
				}
				if binOp.Op != tt.wantOp {
					t.Errorf("Parse(%q).Op = %v, want %v", tt.expr, binOp.Op, tt.wantOp)
				}
			case OpNot:
				unaryOp, ok := node.(*UnaryOpNode)
				if !ok {
					t.Fatalf("Parse(%q) did not return UnaryOpNode, got %T", tt.expr, node)
				}
				if unaryOp.Op != tt.wantOp {
					t.Errorf("Parse(%q).Op = %v, want %v", tt.expr, unaryOp.Op, tt.wantOp)
				}
			}
		})
	}
}

func TestParser_Parse_MembershipOperator(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	node, err := p.Parse(context.Background(), `status in ["active", "pending"]`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	binOp, ok := node.(*BinaryOpNode)
	if !ok {
		t.Fatalf("Parse() did not return BinaryOpNode, got %T", node)
	}
	if binOp.Op != OpIn {
		t.Errorf("Parse().Op = %v, want %v", binOp.Op, OpIn)
	}

	ident, ok := binOp.Left.(*IdentNode)
	if !ok {
		t.Fatalf("Left is not IdentNode, got %T", binOp.Left)
	}
	if ident.Name != "status" {
		t.Errorf("Left.Name = %q, want %q", ident.Name, "status")
	}

	list, ok := binOp.Right.(*ListNode)
	if !ok {
		t.Fatalf("Right is not ListNode, got %T", binOp.Right)
	}
	if len(list.Elements) != 2 {
		t.Errorf("len(Right.Elements) = %d, want 2", len(list.Elements))
	}
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
			node, err := p.Parse(context.Background(), tt.expr)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tt.expr, err)
			}
			call, ok := node.(*CallNode)
			if !ok {
				t.Fatalf("Parse(%q) did not return CallNode, got %T", tt.expr, node)
			}
			if call.Op != tt.wantOp {
				t.Errorf("Parse(%q).Op = %v, want %v", tt.expr, call.Op, tt.wantOp)
			}
			if call.Target == nil {
				t.Error("call.Target is nil")
			}
			if len(call.Args) != 1 {
				t.Errorf("len(call.Args) = %d, want 1", len(call.Args))
			}
		})
	}
}

func TestParser_Parse_SizeFunction(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	node, err := p.Parse(context.Background(), `tags.size()`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	call, ok := node.(*CallNode)
	if !ok {
		t.Fatalf("Parse() did not return CallNode, got %T", node)
	}
	if call.Op != OpSize {
		t.Errorf("Parse().Op = %v, want %v", call.Op, OpSize)
	}
	if call.Target == nil {
		t.Error("call.Target is nil")
	}
}

func TestParser_Parse_HasMacro(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	node, err := p.Parse(context.Background(), `has(user.email)`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	call, ok := node.(*CallNode)
	if !ok {
		t.Fatalf("Parse() did not return CallNode, got %T", node)
	}
	if call.Op != OpHas {
		t.Errorf("Parse().Op = %v, want %v", call.Op, OpHas)
	}
}

func TestParser_Parse_Timestamp(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	node, err := p.Parse(context.Background(), `timestamp("2024-01-15T10:30:00Z")`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	lit, ok := node.(*LiteralNode)
	if !ok {
		t.Fatalf("Parse() did not return LiteralNode, got %T", node)
	}
	ts, ok := lit.Value.(time.Time)
	if !ok {
		t.Fatalf("Value is not time.Time, got %T", lit.Value)
	}
	expected := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	if !ts.Equal(expected) {
		t.Errorf("timestamp = %v, want %v", ts, expected)
	}
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
			node, err := p.Parse(context.Background(), tt.expr)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tt.expr, err)
			}
			if node == nil {
				t.Fatal("Parse() returned nil node")
			}
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
			_, err := p.Parse(context.Background(), tt.expr)
			if err == nil {
				t.Errorf("Parse(%q) expected error, got nil", tt.expr)
			}
		})
	}
}

func TestParser_MustParse_Panic(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustParse() did not panic on invalid expression")
		}
	}()

	p.MustParse(`invalid ===`)
}

func TestParser_MustParse_Success(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	node := p.MustParse(`name == "John"`)
	if node == nil {
		t.Fatal("MustParse() returned nil")
	}
}

func TestParser_Cache(t *testing.T) {
	p, _ := NewParser()

	expr := `name == "John"`

	node1, err := p.Parse(context.Background(), expr)
	if err != nil {
		t.Fatalf("first Parse() error = %v", err)
	}

	node2, err := p.Parse(context.Background(), expr)
	if err != nil {
		t.Fatalf("second Parse() error = %v", err)
	}

	if node1 != node2 {
		t.Error("cache did not return same node instance")
	}
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
			if got := tt.kind.String(); got != tt.want {
				t.Errorf("NodeKind.String() = %q, want %q", got, tt.want)
			}
		})
	}

	// Test unknown kind returns formatted string
	unknown := NodeKind(99)
	if !strings.HasPrefix(unknown.String(), "NodeKind(") {
		t.Errorf("Unknown NodeKind.String() = %q, want prefix 'NodeKind('", unknown.String())
	}
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
			if got := tt.op.String(); got != tt.want {
				t.Errorf("Operator.String() = %q, want %q", got, tt.want)
			}
		})
	}

	// Test unknown operator returns formatted string
	unknown := Operator(99)
	if !strings.HasPrefix(unknown.String(), "Operator(") {
		t.Errorf("Unknown Operator.String() = %q, want prefix 'Operator('", unknown.String())
	}
}

func TestOperator_IsComparison(t *testing.T) {
	comparisons := []Operator{OpEqual, OpNotEqual, OpLT, OpLTE, OpGT, OpGTE}
	for _, op := range comparisons {
		if !op.IsComparison() {
			t.Errorf("%v.IsComparison() = false, want true", op)
		}
	}

	nonComparisons := []Operator{OpAnd, OpOr, OpNot, OpIn, OpContains}
	for _, op := range nonComparisons {
		if op.IsComparison() {
			t.Errorf("%v.IsComparison() = true, want false", op)
		}
	}
}

func TestOperator_IsLogical(t *testing.T) {
	logical := []Operator{OpAnd, OpOr, OpNot}
	for _, op := range logical {
		if !op.IsLogical() {
			t.Errorf("%v.IsLogical() = false, want true", op)
		}
	}

	nonLogical := []Operator{OpEqual, OpIn, OpContains}
	for _, op := range nonLogical {
		if op.IsLogical() {
			t.Errorf("%v.IsLogical() = true, want false", op)
		}
	}
}

func TestOperator_IsStringOp(t *testing.T) {
	stringOps := []Operator{OpContains, OpStartsWith, OpEndsWith, OpMatches}
	for _, op := range stringOps {
		if !op.IsStringOp() {
			t.Errorf("%v.IsStringOp() = false, want true", op)
		}
	}

	nonStringOps := []Operator{OpEqual, OpAnd, OpIn, OpSize}
	for _, op := range nonStringOps {
		if op.IsStringOp() {
			t.Errorf("%v.IsStringOp() = true, want false", op)
		}
	}
}

func TestWalk(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())
	node, _ := p.Parse(context.Background(), `name == "John" && age >= 18`)

	count := 0
	Walk(node, func(n Node) bool {
		count++
		return true
	})

	if count == 0 {
		t.Error("Walk did not visit any nodes")
	}
}

func TestAllNodes(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())
	node, _ := p.Parse(context.Background(), `name == "John"`)

	count := 0
	for range AllNodes(node) {
		count++
	}

	if count != CountNodes(node) {
		t.Errorf("AllNodes count = %d, CountNodes = %d", count, CountNodes(node))
	}
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
			node, _ := p.Parse(context.Background(), tt.expr)
			d := Depth(node)
			if d < tt.minDepth {
				t.Errorf("Depth(%q) = %d, want >= %d", tt.expr, d, tt.minDepth)
			}
		})
	}

	// Test nil
	if Depth(nil) != 0 {
		t.Error("Depth(nil) should be 0")
	}
}

func TestParser_Parse_ExpressionTooLong(t *testing.T) {
	p, _ := NewParser(WithParserNoCache(), WithMaxExpressionLength(50))

	t.Run("expression within limit", func(t *testing.T) {
		_, err := p.Parse(context.Background(), `name == "John"`)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
	})

	t.Run("expression exceeding limit", func(t *testing.T) {
		long := `name == "` + strings.Repeat("a", 50) + `"`
		_, err := p.Parse(context.Background(), long)
		if !errors.Is(err, ErrExpressionTooLong) {
			t.Errorf("Parse() error = %v, want %v", err, ErrExpressionTooLong)
		}
	})

	t.Run("default limit allows reasonable expressions", func(t *testing.T) {
		pDefault, _ := NewParser(WithParserNoCache())
		_, err := pDefault.Parse(context.Background(), `name == "John" && age >= 18`)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
	})
}

func TestValidateRegex(t *testing.T) {
	t.Run("valid short regex", func(t *testing.T) {
		if err := ValidateRegex("^hello.*", 1024); err != nil {
			t.Errorf("ValidateRegex() error = %v", err)
		}
	})

	t.Run("regex exceeding length", func(t *testing.T) {
		long := strings.Repeat("a", 1025)
		err := ValidateRegex(long, 1024)
		if !errors.Is(err, ErrInvalidRegex) {
			t.Errorf("ValidateRegex() error = %v, want %v", err, ErrInvalidRegex)
		}
	})

	t.Run("invalid regex pattern", func(t *testing.T) {
		err := ValidateRegex("[invalid", 1024)
		if !errors.Is(err, ErrInvalidRegex) {
			t.Errorf("ValidateRegex() error = %v, want %v", err, ErrInvalidRegex)
		}
	})
}

func TestParser_Parse_DefaultExpressionLength(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	// Build an expression that exceeds DefaultMaxExpressionLength (4096)
	long := `name == "` + strings.Repeat("x", DefaultMaxExpressionLength) + `"`
	_, err := p.Parse(context.Background(), long)
	if !errors.Is(err, ErrExpressionTooLong) {
		t.Errorf("Parse() error = %v, want %v", err, ErrExpressionTooLong)
	}
}

func TestNode_Children(t *testing.T) {
	p, _ := NewParser(WithParserNoCache())

	t.Run("LiteralNode has no children", func(t *testing.T) {
		node, _ := p.Parse(context.Background(), `"hello"`)
		count := 0
		for range node.Children() {
			count++
		}
		if count != 0 {
			t.Errorf("LiteralNode has %d children, want 0", count)
		}
	})

	t.Run("BinaryOpNode has 2 children", func(t *testing.T) {
		node, _ := p.Parse(context.Background(), `name == "John"`)
		count := 0
		for range node.Children() {
			count++
		}
		if count != 2 {
			t.Errorf("BinaryOpNode has %d children, want 2", count)
		}
	})

	t.Run("ListNode has children for each element", func(t *testing.T) {
		node, _ := p.Parse(context.Background(), `status in ["a", "b", "c"]`)
		binOp := node.(*BinaryOpNode)
		list := binOp.Right.(*ListNode)
		count := 0
		for range list.Children() {
			count++
		}
		if count != 3 {
			t.Errorf("ListNode has %d children, want 3", count)
		}
	})
}
