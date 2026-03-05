// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package filesystem provides a PolicySource implementation that reads OPA policies
// from the local filesystem. Change detection is handled by the Manager.
//
// Example usage:
//
//	source, err := filesystem.New("/path/to/policies",
//	    filesystem.WithExtensions(".rego"),
//	    filesystem.WithIncludeData(true),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer source.Close()
//
//	bundle, err := source.Fetch(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
package filesystem
