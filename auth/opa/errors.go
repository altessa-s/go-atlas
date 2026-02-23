// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import "errors"

// Sentinel errors for the OPA package.
var (
	// ErrSourceRequired indicates that a policy source is required but was not provided.
	ErrSourceRequired = errors.New("policy source is required")

	// ErrQueryRequired indicates that a Rego query is required but was not provided.
	ErrQueryRequired = errors.New("query is required")

	// ErrPoliciesNotLoaded indicates that policies have not been loaded yet.
	ErrPoliciesNotLoaded = errors.New("policies not loaded")

	// ErrSourceClosed indicates that the policy source has been closed.
	ErrSourceClosed = errors.New("source is closed")

	// ErrManagerClosed indicates that the manager has been closed.
	ErrManagerClosed = errors.New("manager is closed")

	// ErrNoPolicyFiles indicates that no policy files were found in the specified path.
	ErrNoPolicyFiles = errors.New("no policy files found")

	// ErrBundleFetchFailed indicates that fetching the policy bundle failed.
	ErrBundleFetchFailed = errors.New("bundle fetch failed")

	// ErrQueryPrepareFailed indicates that preparing the Rego query failed.
	ErrQueryPrepareFailed = errors.New("query preparation failed")

	// ErrWatchStartFailed indicates that starting the policy watch failed.
	ErrWatchStartFailed = errors.New("watch start failed")

	// ErrSchedulerManaged indicates that the function is managed by a scheduler
	// and direct calls are not allowed.
	ErrSchedulerManaged = errors.New("function is managed by scheduler, direct calls not allowed")
)
