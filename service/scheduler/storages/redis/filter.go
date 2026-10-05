// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"maps"
	"slices"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/filter/translators/redisearch"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// taskExactFields and historyExactFields are the NUMERIC index fields every
// stored document carries (no omitempty in taskData / historyData, and every
// Lua script writes them back), so a RediSearch range over them selects
// exactly the documents the CEL comparison does.
var (
	taskExactFields    = coremaps.NewImmutableMap(map[string]struct{}{"status": {}, "priority": {}, "failures": {}})
	historyExactFields = coremaps.NewImmutableMap(map[string]struct{}{"startedAt": {}, "endedAt": {}, "durationMs": {}})
)

// filterPlan is a filter prepared for the Redis backend. RediSearch does not
// evaluate CEL: TAG fields fold case, TEXT fields are tokenized, an omitted
// zero field is absent from the index, and endsWith, matches and size have no
// query form. So the whole filter is evaluated on the client (match), and
// only the part RediSearch evaluates identically is pushed down (query) to
// narrow what is fetched.
type filterPlan struct {
	evaluator *filter.Evaluator
	node      filter.Node
	query     string // pushed-down RediSearch query; "" when nothing is pushed down
}

// newFilterPlan prepares f, which must be non-nil. exact names the fields
// whose numeric comparisons may be pushed down. Every field f references is
// checked against allowed up front, so a filter naming another field fails
// here rather than only once — and only if — a fetched document reaches it.
func newFilterPlan(
	f filter.Node,
	allowed []string,
	schema *coremaps.ImmutableMap[string, redisearch.FieldType],
	exact *coremaps.ImmutableMap[string, struct{}],
	opts ...filter.TranslatorOption,
) (*filterPlan, error) {
	if err := checkFields(f, allowed); err != nil {
		return nil, err
	}
	evaluator, err := filter.NewEvaluator(filter.WithAllowedFields(allowed...))
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "build filter evaluator")
	}
	plan := &filterPlan{evaluator: evaluator, node: f}
	pushed := pushdown(f, exact)
	if pushed == nil {
		return plan, nil
	}
	trans, err := redisearch.NewTranslator(maps.Collect(schema.All()), append(opts, filter.WithAllowedFields(allowed...))...)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "build filter translator")
	}
	if plan.query, err = trans.Translate(pushed); err != nil {
		return nil, coreerrs.WrapOperation(err, "translate filter")
	}
	return plan, nil
}

// checkFields fails with [filter.ErrFieldNotAllowed] when f references a field
// outside allowed, wherever the reference sits — including behind a branch the
// evaluator would short-circuit past.
func checkFields(f filter.Node, allowed []string) error {
	var err error
	filter.Walk(f, func(n filter.Node) bool {
		if ident, ok := n.(*filter.IdentNode); ok && err == nil && !slices.Contains(allowed, ident.Name) {
			err = coreerrs.Wrapf(filter.ErrFieldNotAllowed, "%s", ident.Name)
		}
		return err == nil
	})
	return err
}

// match reports whether the document described by data satisfies the filter.
func (p *filterPlan) match(data map[string]any) (bool, error) {
	ok, err := p.evaluator.Evaluate(p.node, data)
	if err != nil {
		return false, coreerrs.WrapOperation(err, "evaluate filter")
	}
	return ok, nil
}

// pushdown returns the conjuncts of node that RediSearch evaluates exactly,
// joined by &&, or nil when there are none. Only the top-level && chain is
// split: a conjunct that is not exact is left to the client, which narrows
// the result no further than the pushed-down part allows.
func pushdown(node filter.Node, exact *coremaps.ImmutableMap[string, struct{}]) filter.Node {
	if and, ok := node.(*filter.BinaryOpNode); ok && and.Op == filter.OpAnd {
		left, right := pushdown(and.Left, exact), pushdown(and.Right, exact)
		switch {
		case left == nil:
			return right
		case right == nil:
			return left
		}
		return &filter.BinaryOpNode{Op: filter.OpAnd, Left: left, Right: right}
	}
	if isExact(node, exact) {
		return node
	}
	return nil
}

// isExact reports whether node is built only from comparisons and non-empty
// memberships between an exact numeric field and numeric literals, combined
// with &&, || and !. An empty membership is excluded: the translator renders
// it as an invalid query.
func isExact(node filter.Node, exact *coremaps.ImmutableMap[string, struct{}]) bool {
	switch n := node.(type) {
	case *filter.BinaryOpNode:
		switch n.Op {
		case filter.OpAnd, filter.OpOr:
			return isExact(n.Left, exact) && isExact(n.Right, exact)
		case filter.OpEqual, filter.OpNotEqual, filter.OpLT, filter.OpLTE, filter.OpGT, filter.OpGTE:
			return isExactField(n.Left, exact) && isNumericLiteral(n.Right)
		case filter.OpIn:
			list, ok := n.Right.(*filter.ListNode)
			if !ok || len(list.Elements) == 0 || !isExactField(n.Left, exact) {
				return false
			}
			for _, e := range list.Elements {
				if !isNumericLiteral(e) {
					return false
				}
			}
			return true
		default:
			return false
		}
	case *filter.UnaryOpNode:
		return n.Op == filter.OpNot && isExact(n.Operand, exact)
	default:
		return false
	}
}

func isExactField(node filter.Node, exact *coremaps.ImmutableMap[string, struct{}]) bool {
	ident, ok := node.(*filter.IdentNode)
	return ok && exact.Contains(ident.Name)
}

func isNumericLiteral(node filter.Node) bool {
	lit, ok := node.(*filter.LiteralNode)
	if !ok {
		return false
	}
	switch lit.Value.(type) {
	case int64, uint64, float64:
		return true
	default:
		return false
	}
}
