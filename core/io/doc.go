// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package io provides I/O utilities: buffer pooling, byte-limited readers, and
// a lazy range-based read-seeker for remote objects. For one-shot
// materialization of streaming sources see the sibling [spool] package.
//
// # Usage
//
//	buf := io.GetBuffer()
//	defer io.PutBuffer(buf)
//
//	lr := io.NewLimitedReadCloser(rc, 4<<20) // 4 MiB limit
//	defer lr.Close()
//
//	rs := io.NewRangeReadSeeker(ctx, objectSize, opener)
//	defer rs.Close()
//
// [spool]: https://pkg.go.dev/github.com/altessa-s/go-atlas/core/io/spool
package io
