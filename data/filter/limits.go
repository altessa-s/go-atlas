// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"github.com/altessa-s/go-atlas/core/types/nilcheck"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// CheckLimits bounds a complete translation before any output is built. Every
// node consumes an operation; compound nodes, including lists, consume depth.
// A nil root is an absent filter and consumes no budget. Unlike evaluation,
// translation visits both branches of a logical expression.
func (o *translatorOptions) CheckLimits(root Node) error {
	if root == nil {
		return nil
	}
	remaining := o.maxOperations
	return o.checkNodeLimits(root, 0, &remaining)
}

func (o *translatorOptions) checkNodeLimits(node Node, depth int, remaining *int) error {
	if nilcheck.IsNil(node) {
		return coreerrs.Wrap(ErrInvalidExpression, "nil AST node")
	}
	if *remaining == 0 {
		return ErrMaxOperationsExceeded
	}
	*remaining--
	switch node.(type) {
	case *LiteralNode, *IdentNode:
		return nil
	}
	if depth >= o.maxDepth {
		return ErrMaxDepthExceeded
	}
	for child := range node.Children() {
		if err := o.checkNodeLimits(child, depth+1, remaining); err != nil {
			return err
		}
	}
	return nil
}
