// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"reflect"
	"slices"
	"time"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// cloneAST copies the built-in AST and mutable byte literals. Unknown custom
// nodes and mutable literal types fail explicitly instead of aliasing the cache.
func cloneAST(root Node) (Node, error) {
	return cloneNode(root, make(map[Node]Node))
}

//nolint:nilnil // A nil child represents an absent target in built-in call nodes.
func cloneNode(node Node, copies map[Node]Node) (Node, error) {
	if node == nil {
		return nil, nil
	}
	var dst Node
	switch n := node.(type) {
	case *LiteralNode:
		if n == nil {
			return nil, coreerrs.Wrapf(ErrInvalidExpression, "nil node %T", node)
		}
		value, err := cloneLiteral(n.Value)
		if err != nil {
			return nil, err
		}
		return &LiteralNode{Value: value, Text: n.Text}, nil
	case *IdentNode:
		if n == nil {
			return nil, coreerrs.Wrapf(ErrInvalidExpression, "nil node %T", node)
		}
		return &IdentNode{Name: n.Name}, nil
	case *BinaryOpNode:
		if n == nil {
			return nil, coreerrs.Wrapf(ErrInvalidExpression, "nil node %T", node)
		}
		dst = &BinaryOpNode{Op: n.Op}
	case *UnaryOpNode:
		if n == nil {
			return nil, coreerrs.Wrapf(ErrInvalidExpression, "nil node %T", node)
		}
		dst = &UnaryOpNode{Op: n.Op}
	case *CallNode:
		if n == nil {
			return nil, coreerrs.Wrapf(ErrInvalidExpression, "nil node %T", node)
		}
		dst = &CallNode{Op: n.Op}
	case *ListNode:
		if n == nil {
			return nil, coreerrs.Wrapf(ErrInvalidExpression, "nil node %T", node)
		}
		dst = &ListNode{}
	default:
		return nil, coreerrs.Wrapf(ErrInvalidExpression, "cannot copy custom node %T", node)
	}
	if previous, ok := copies[node]; ok {
		return previous, nil
	}
	copies[node] = dst
	var err error
	switch n := node.(type) {
	case *BinaryOpNode:
		out, _ := dst.(*BinaryOpNode) // constructed by the matching case above
		out.Left, err = cloneNode(n.Left, copies)
		if err == nil {
			out.Right, err = cloneNode(n.Right, copies)
		}
	case *UnaryOpNode:
		out, _ := dst.(*UnaryOpNode) // constructed by the matching case above
		out.Operand, err = cloneNode(n.Operand, copies)
	case *CallNode:
		out, _ := dst.(*CallNode) // constructed by the matching case above
		out.Target, err = cloneNode(n.Target, copies)
		if err == nil {
			out.Args, err = cloneNodes(n.Args, copies)
		}
	case *ListNode:
		out, _ := dst.(*ListNode) // constructed by the matching case above
		out.Elements, err = cloneNodes(n.Elements, copies)
	}
	return dst, err
}

func cloneNodes(nodes []Node, copies map[Node]Node) ([]Node, error) {
	if nodes == nil {
		return nil, nil
	}
	out := make([]Node, len(nodes))
	for i, node := range nodes {
		var err error
		out[i], err = cloneNode(node, copies)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func cloneLiteral(value any) (any, error) {
	switch v := value.(type) {
	case nil, time.Time:
		return v, nil
	case []byte:
		return slices.Clone(v), nil
	}
	kind := reflect.TypeOf(value).Kind()
	if kind == reflect.String || (kind >= reflect.Bool && kind <= reflect.Complex128) {
		return value, nil
	}
	return nil, coreerrs.Wrapf(ErrUnsupportedType, "cannot copy literal %T", value)
}
