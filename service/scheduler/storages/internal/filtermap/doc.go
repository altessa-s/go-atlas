// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package filtermap renders scheduler task states and history entries as the
// field maps a [filter.Evaluator] reads, keyed by the CEL names of
// [scheduler.TaskFilterFields] and [scheduler.HistoryFilterFields]. The
// storages that evaluate filters on the client — memory, and Redis for the
// predicates RediSearch cannot evaluate exactly — share it, so a filter means
// the same thing on both.
//
// # Usage
//
//	match, err := evaluator.Evaluate(node, filtermap.Task(state))
//
// [filter.Evaluator]: https://pkg.go.dev/github.com/altessa-s/go-atlas/data/filter#Evaluator
package filtermap
