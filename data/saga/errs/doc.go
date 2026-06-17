// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package errs holds the sentinel errors returned by the saga orchestrator and
// its storage backends. They live in a dedicated package so that backend
// implementations (in data/saga/storages/*) can return them, and callers can
// match them with [errors.Is], without importing the parent saga package.
package errs
