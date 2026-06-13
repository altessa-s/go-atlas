// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package spool materializes a streaming [io.Reader] into a local, rewindable
// backing store — memory for small content, a temp file for larger content —
// so callers can re-read it from offset 0 without touching the origin again.
//
// # Usage
//
//	sp, err := spool.New(r, spool.WithMemThreshold(4<<20))
//	if err != nil { /* handle */ }
//	defer sp.Close()
//
//	data, err := io.ReadAll(sp)
//
//	// Rewind and read again.
//	_, err = sp.Seek(0, io.SeekStart)
package spool
