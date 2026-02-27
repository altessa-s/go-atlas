// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/filter"
)

func TestNodeKinds(t *testing.T) {
	tests := []struct {
		node filter.Node
		want filter.NodeKind
	}{
		{&filter.LiteralNode{Value: 42}, filter.NodeKindLiteral},
		{&filter.IdentNode{Name: "x"}, filter.NodeKindIdent},
		{&filter.BinaryOpNode{Op: filter.OpEqual}, filter.NodeKindBinaryOp},
		{&filter.UnaryOpNode{Op: filter.OpNot}, filter.NodeKindUnaryOp},
		{&filter.CallNode{Op: filter.OpContains}, filter.NodeKindCall},
		{&filter.ListNode{}, filter.NodeKindList},
	}
	for _, tt := range tests {
		if got := tt.node.Kind(); got != tt.want {
			t.Errorf("Kind() = %v, want %v", got, tt.want)
		}
	}
}

func TestUnaryOpNode_Children(t *testing.T) {
	child := &filter.LiteralNode{Value: true}
	node := &filter.UnaryOpNode{Op: filter.OpNot, Operand: child}
	count := 0
	for range node.Children() {
		count++
	}
	if count != 1 {
		t.Errorf("UnaryOpNode children count = %d, want 1", count)
	}
}

func TestCallNode_Children(t *testing.T) {
	target := &filter.IdentNode{Name: "name"}
	arg := &filter.LiteralNode{Value: "hello"}
	node := &filter.CallNode{Op: filter.OpContains, Target: target, Args: []filter.Node{arg}}
	count := 0
	for range node.Children() {
		count++
	}
	if count != 2 {
		t.Errorf("CallNode children count = %d, want 2", count)
	}
}

func TestTranslatorConfig_MaxDepth(t *testing.T) {
	cfg := filter.NewTranslatorConfig()
	filter.WithMaxDepth(5)(cfg)
	if cfg.MaxDepth() != 5 {
		t.Errorf("MaxDepth() = %d, want 5", cfg.MaxDepth())
	}
}

func TestTranslatorConfig_StrictMode(t *testing.T) {
	cfg := filter.NewTranslatorConfig()
	filter.WithStrictMode(true)(cfg)
	if !cfg.StrictMode() {
		t.Error("StrictMode() should be true")
	}
}

func TestTranslatorConfig_SetAllowedFields(t *testing.T) {
	cfg := filter.NewTranslatorConfig()
	cfg.SetAllowedFields(map[string]struct{}{"name": {}, "age": {}})
	if !cfg.IsFieldAllowed("name") {
		t.Error("name should be allowed")
	}
	if cfg.IsFieldAllowed("other") {
		t.Error("other should not be allowed")
	}
}

func TestTranslatorConfig_SetFieldMapping(t *testing.T) {
	cfg := filter.NewTranslatorConfig()
	cfg.SetFieldMapping(map[string]string{"name": "full_name"})
	if got := cfg.ApplyFieldMapping("name"); got != "full_name" {
		t.Errorf("ApplyFieldMapping(name) = %q, want full_name", got)
	}
}

func TestEvaluator_NilNotEqual(t *testing.T) {
	parser, _ := filter.NewParser()
	node, _ := parser.Parse(t.Context(), "name != null")
	ev := filter.NewEvaluator()

	result, err := ev.Evaluate(node, map[string]any{"name": "hello"})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result {
		t.Error("'hello' != null should be true")
	}
}
