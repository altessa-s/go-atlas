// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package txscope

import (
	"context"
	"sync/atomic"
)

// Key marks a private context key as transactional. Embed it so a detached
// callback cannot see the value belonging to the completed scope.
type Key struct{}

func (Key) transactionScoped() {}

type commitReportingKey struct{ Key }

// WithCommitReporting asks the owning transaction adapter to return
// post-commit failures as errors rather than propagate an observer panic.
func WithCommitReporting(ctx context.Context) context.Context {
	return context.WithValue(ctx, commitReportingKey{}, true)
}

// StartCommitReport binds confirmation to one invocation of a transaction
// adapter. Give the returned context to its body. A nil confirm means the
// caller did not request a report or an enclosing adapter already claimed it.
func StartCommitReport(ctx context.Context) (context.Context, func()) {
	if requested, _ := ctx.Value(commitReportingKey{}).(bool); !requested {
		return ctx, nil
	}
	report := new(CommitReport)
	return context.WithValue(ctx, commitReportingKey{}, report), func() { report.confirmed.Store(true) }
}

// CommitReport records the outcome of the adapter that supplied the body context.
type CommitReport struct{ confirmed atomic.Bool }

// Confirmed reports whether the adapter confirmed the commit.
func (r *CommitReport) Confirmed() bool { return r != nil && r.confirmed.Load() }

// ReportFrom returns the report attached to the body context by its adapter.
func ReportFrom(ctx context.Context) *CommitReport {
	report, _ := ctx.Value(commitReportingKey{}).(*CommitReport)
	return report
}

// Detach hides transaction-scoped values from a callback that runs after
// commit. Other values and the cancellation policy come from ctx.
func Detach(ctx context.Context) context.Context {
	return detachedContext{Context: ctx}
}

type detachedContext struct{ context.Context }

func (c detachedContext) Value(key any) any {
	if _, scoped := key.(interface{ transactionScoped() }); scoped {
		return nil
	}
	return c.Context.Value(key)
}
