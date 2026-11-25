// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storagetest is the conformance suite of audit storages: every
// backend under audit/storages runs [Run] to prove it implements the
// batch input contract and the order, paging and Count rules of [audit.FetchPage].
// [BatchFailure] exercises backend-specific partial writes.
package storagetest
