// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
)

// errVisit is what recordingVisitor returns for the value it refuses.
var errVisit = errors.New("visit failed")

// recordingVisitor is the minimum Visitor that VisitElements needs: it
// only ever sees literals and lists, and returns each value unchanged.
// failOn names the one value it refuses, so the abort path is reachable.
type recordingVisitor struct {
	failOn any
	seen   []any
}

func (v *recordingVisitor) VisitLiteral(n *filter.LiteralNode) (any, error) {
	if v.failOn != nil && n.Value == v.failOn {
		return nil, errVisit
	}
	v.seen = append(v.seen, n.Value)
	return n.Value, nil
}

func (v *recordingVisitor) VisitList(n *filter.ListNode) (any, error) {
	return filter.VisitElements(v, n)
}

func (*recordingVisitor) VisitIdent(*filter.IdentNode) (any, error)       { return nil, errVisit }
func (*recordingVisitor) VisitBinaryOp(*filter.BinaryOpNode) (any, error) { return nil, errVisit }
func (*recordingVisitor) VisitUnaryOp(*filter.UnaryOpNode) (any, error)   { return nil, errVisit }
func (*recordingVisitor) VisitCall(*filter.CallNode) (any, error)         { return nil, errVisit }

// literals wraps plain values as literal nodes.
func literals(values ...any) []filter.Node {
	nodes := make([]filter.Node, 0, len(values))
	for _, v := range values {
		nodes = append(nodes, &filter.LiteralNode{Value: v})
	}
	return nodes
}

func TestVisitElements(t *testing.T) {
	t.Parallel()

	t.Run("returns the element values in order", func(t *testing.T) {
		t.Parallel()

		v := &recordingVisitor{}
		got, err := filter.VisitElements(v, &filter.ListNode{
			Elements: literals(int64(1), "two", nil, true),
		})
		require.NoError(t, err)
		require.Equal(t, []any{int64(1), "two", nil, true}, got)
	})

	t.Run("empty list", func(t *testing.T) {
		t.Parallel()

		v := &recordingVisitor{}
		got, err := filter.VisitElements(v, &filter.ListNode{})
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("aborts on the first failing element", func(t *testing.T) {
		t.Parallel()

		v := &recordingVisitor{failOn: "bad"}
		_, err := filter.VisitElements(v, &filter.ListNode{
			Elements: literals(int64(1), "bad", int64(3)),
		})
		require.ErrorIs(t, err, errVisit)
		require.Equal(t, []any{int64(1)}, v.seen, "the walk must stop at the failure, not run on")
	})

	t.Run("nested lists recurse through the visitor", func(t *testing.T) {
		t.Parallel()

		v := &recordingVisitor{}
		got, err := filter.VisitElements(v, &filter.ListNode{
			Elements: []filter.Node{
				&filter.LiteralNode{Value: int64(1)},
				&filter.ListNode{Elements: literals(int64(2), int64(3))},
			},
		})
		require.NoError(t, err)
		require.Equal(t, []any{int64(1), []any{int64(2), int64(3)}}, got)
	})
}
