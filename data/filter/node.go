// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import "iter"

//go:generate stringer -type=NodeKind,Operator -output=node_string.go

// NodeKind represents the type of AST node.
type NodeKind uint8

const (
	// NodeKindLiteral represents literal values: "John", 18, true, nil.
	NodeKindLiteral NodeKind = iota
	// NodeKindIdent represents identifiers: name, address.city.
	NodeKindIdent
	// NodeKindBinaryOp represents binary operations: ==, !=, <, >, <=, >=, &&, ||, in.
	NodeKindBinaryOp
	// NodeKindUnaryOp represents unary operations: !.
	NodeKindUnaryOp
	// NodeKindCall represents function calls: contains(), startsWith(), size(), matches().
	NodeKindCall
	// NodeKindList represents list literals: [1, 2, 3].
	NodeKindList
)

// Operator represents operators used in expressions.
type Operator uint8

const (
	// OpEqual is the equality comparison operator (==).
	OpEqual Operator = iota
	// OpNotEqual is the inequality comparison operator (!=).
	OpNotEqual
	// OpLT is the less-than comparison operator (<).
	OpLT
	// OpLTE is the less-than-or-equal comparison operator (<=).
	OpLTE
	// OpGT is the greater-than comparison operator (>).
	OpGT
	// OpGTE is the greater-than-or-equal comparison operator (>=).
	OpGTE

	// OpAnd is the logical AND operator (&&).
	OpAnd
	// OpOr is the logical OR operator (||).
	OpOr
	// OpNot is the logical NOT operator (!).
	OpNot

	// OpIn is the membership test operator (in).
	OpIn

	// OpContains tests whether a string contains a substring.
	OpContains
	// OpStartsWith tests whether a string begins with a prefix.
	OpStartsWith
	// OpEndsWith tests whether a string ends with a suffix.
	OpEndsWith
	// OpMatches tests whether a string matches a regular expression.
	OpMatches

	// OpSize returns the length of a collection.
	OpSize
	// OpHas tests whether a field exists on the target.
	OpHas
	// OpExists is an alias for [OpHas].
	OpExists
)

// IsComparison returns true if the operator is a comparison operator.
func (o Operator) IsComparison() bool {
	return o >= OpEqual && o <= OpGTE
}

// IsLogical returns true if the operator is a logical operator.
func (o Operator) IsLogical() bool {
	return o >= OpAnd && o <= OpNot
}

// IsStringOp returns true if the operator is a string operation.
func (o Operator) IsStringOp() bool {
	return o >= OpContains && o <= OpMatches
}

// Node represents a node in the filter AST.
type Node interface {
	// Kind returns the type of this node.
	Kind() NodeKind
	// Accept allows a visitor to process this node.
	Accept(Visitor) (any, error)
	// Children returns an iterator over child nodes.
	Children() iter.Seq[Node]
}

// LiteralNode represents a literal value (string, number, bool, nil).
type LiteralNode struct {
	Value any
}

func (n *LiteralNode) Kind() NodeKind                { return NodeKindLiteral }
func (n *LiteralNode) Accept(v Visitor) (any, error) { return v.VisitLiteral(n) }
func (n *LiteralNode) Children() iter.Seq[Node]      { return func(yield func(Node) bool) {} }

// IdentNode represents an identifier or field reference.
type IdentNode struct {
	Name string // Field name, may include dots for nested fields (e.g., "address.city")
}

func (n *IdentNode) Kind() NodeKind                { return NodeKindIdent }
func (n *IdentNode) Accept(v Visitor) (any, error) { return v.VisitIdent(n) }
func (n *IdentNode) Children() iter.Seq[Node]      { return func(yield func(Node) bool) {} }

// BinaryOpNode represents a binary operation (comparison, logical, membership).
type BinaryOpNode struct {
	Op    Operator
	Left  Node
	Right Node
}

func (n *BinaryOpNode) Kind() NodeKind                { return NodeKindBinaryOp }
func (n *BinaryOpNode) Accept(v Visitor) (any, error) { return v.VisitBinaryOp(n) }
func (n *BinaryOpNode) Children() iter.Seq[Node] {
	return func(yield func(Node) bool) {
		if n.Left != nil && !yield(n.Left) {
			return
		}
		if n.Right != nil {
			yield(n.Right)
		}
	}
}

// UnaryOpNode represents a unary operation (negation).
type UnaryOpNode struct {
	Op      Operator
	Operand Node
}

func (n *UnaryOpNode) Kind() NodeKind                { return NodeKindUnaryOp }
func (n *UnaryOpNode) Accept(v Visitor) (any, error) { return v.VisitUnaryOp(n) }
func (n *UnaryOpNode) Children() iter.Seq[Node] {
	return func(yield func(Node) bool) {
		if n.Operand != nil {
			yield(n.Operand)
		}
	}
}

// CallNode represents a function/method call.
type CallNode struct {
	Op     Operator // The operation being performed (OpContains, OpStartsWith, etc.)
	Target Node     // The target of the call (for method calls like name.contains())
	Args   []Node   // Arguments to the function
}

func (n *CallNode) Kind() NodeKind                { return NodeKindCall }
func (n *CallNode) Accept(v Visitor) (any, error) { return v.VisitCall(n) }
func (n *CallNode) Children() iter.Seq[Node] {
	return func(yield func(Node) bool) {
		if n.Target != nil && !yield(n.Target) {
			return
		}
		for _, arg := range n.Args {
			if !yield(arg) {
				return
			}
		}
	}
}

// ListNode represents a list literal.
type ListNode struct {
	Elements []Node
}

func (n *ListNode) Kind() NodeKind                { return NodeKindList }
func (n *ListNode) Accept(v Visitor) (any, error) { return v.VisitList(n) }
func (n *ListNode) Children() iter.Seq[Node] {
	return func(yield func(Node) bool) {
		for _, elem := range n.Elements {
			if !yield(elem) {
				return
			}
		}
	}
}

// Walk traverses the AST in depth-first order, calling fn for each node.
// If fn returns false, traversal stops.
func Walk(root Node, fn func(Node) bool) {
	if root == nil {
		return
	}
	if !fn(root) {
		return
	}
	for child := range root.Children() {
		Walk(child, fn)
	}
}

// AllNodes returns an iterator over all nodes in the AST in depth-first order.
func AllNodes(root Node) iter.Seq[Node] {
	return func(yield func(Node) bool) {
		Walk(root, yield)
	}
}

// CountNodes returns the total number of nodes in the AST.
func CountNodes(root Node) int {
	count := 0
	Walk(root, func(Node) bool {
		count++
		return true
	})
	return count
}

// Depth returns the maximum depth of the AST.
func Depth(root Node) int {
	if root == nil {
		return 0
	}
	maxChildDepth := 0
	for child := range root.Children() {
		if d := Depth(child); d > maxChildDepth {
			maxChildDepth = d
		}
	}
	return maxChildDepth + 1
}
