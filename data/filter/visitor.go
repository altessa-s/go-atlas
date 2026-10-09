// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import coreerrs "github.com/altessa-s/go-atlas/core/errors"

// Visitor defines the interface for traversing the filter AST.
// Implementations translate the AST into target representations (e.g., MongoDB bson.M).
type Visitor interface {
	// VisitLiteral processes a literal value node.
	VisitLiteral(*LiteralNode) (any, error)
	// VisitIdent processes an identifier node.
	VisitIdent(*IdentNode) (any, error)
	// VisitBinaryOp processes a binary operation node.
	VisitBinaryOp(*BinaryOpNode) (any, error)
	// VisitUnaryOp processes a unary operation node.
	VisitUnaryOp(*UnaryOpNode) (any, error)
	// VisitCall processes a function call node.
	VisitCall(*CallNode) (any, error)
	// VisitList processes a list literal node.
	VisitList(*ListNode) (any, error)
}

// VisitElements accepts every element of a list node through v and
// collects the results in order.
//
// It is the whole of what [Visitor.VisitList] has to do: a list literal
// carries no semantics of its own, only its elements, so every
// translator in this repository implemented the same loop. Implementers
// can delegate to it:
//
//	func (t *Translator) VisitList(n *filter.ListNode) (any, error) {
//	    return filter.VisitElements(t, n)
//	}
//
// The first element that fails aborts the walk and its error is
// returned unwrapped.
func VisitElements(v Visitor, n *ListNode) ([]any, error) {
	result := make([]any, 0, len(n.Elements))
	for _, elem := range n.Elements {
		val, err := elem.Accept(v)
		if err != nil {
			return nil, err
		}
		result = append(result, val)
	}
	return result, nil
}

// AcceptLiteral visits a literal operand, rejecting expressions that a backend
// cannot evaluate in value position. In particular, a rendered field name must
// never be mistaken for a string literal.
func AcceptLiteral(v Visitor, node Node) (any, error) {
	if literal, ok := node.(*LiteralNode); ok && literal != nil {
		return literal.Accept(v)
	}
	return nil, coreerrs.Wrapf(ErrUnsupportedOperation, "expected literal operand, got %T", node)
}

// VisitLiteralElements visits a list whose backend requires constant members.
// Dynamic members are rejected rather than converted to their rendered names.
func VisitLiteralElements(v Visitor, n *ListNode) ([]any, error) {
	result := make([]any, 0, len(n.Elements))
	for _, elem := range n.Elements {
		value, err := AcceptLiteral(v, elem)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}
