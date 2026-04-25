// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

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
