// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storagetest is the conformance suite of audit storages: every
// backend under data/audit/storages runs [Run] to prove it implements the
// ordering, paging and Count contract that [audit.FetchPage] relies on.
package storagetest
