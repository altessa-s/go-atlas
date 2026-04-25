// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package id provides stable, unique Service ID generation and management for
// distributed applications.
//
// A Service ID uniquely identifies a running service instance across restarts
// and deployments. The package offers three built-in [Provider] implementations:
//
//   - [Env] reads the ID from an environment variable.
//   - [File] reads or generates an ID persisted to a file on disk.
//   - [Static] wraps a caller-supplied constant string.
//
// The top-level constructor [New] implements a cascading strategy: it first
// attempts to read the ID from an environment variable, and falls back to
// file-based persistence when the variable is unset or empty.
//
// Every constructor has a Must variant (e.g. [MustNew], [MustNewWithFileProvider])
// that panics instead of returning an error, which is convenient for
// program-level initialization where failure is unrecoverable.
//
// Example:
//
//	// Auto-select provider (env var first, then file)
//	s, err := id.New("/tmp/service.id", "SERVICE_ID")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("Service ID:", s.ID())
//
//	// Use a specific provider
//	fileID, err := id.NewWithFileProvider("/var/lib/app/service.id")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Must variants panic on error
//	s := id.MustNew("/tmp/service.id", "SERVICE_ID")
package id
