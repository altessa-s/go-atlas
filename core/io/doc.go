// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package io provides I/O utilities including buffer pooling and limited readers.
//
// # Usage
//
//	buf := io.GetBuffer()
//	defer io.PutBuffer(buf)
//
//	lr := io.NewLimitedReadCloser(rc, 1024*1024) // 1MB limit
//	defer lr.Close()
package io
