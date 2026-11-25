// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package filesystem provides a PolicySource implementation that reads OPA policies
// from the local filesystem. Change detection is handled by the Manager.
// JSON data files preserve number precision.
// # Release manifests
//
// Each fetch verifies a complete manifest before returning a bundle.
// Use WithReleaseManifest for a JSON file read on every fetch or WithChecksums for a fixed release.
// For directory rollouts, the manifest's root selects a release directory and its files map lists
// SHA-256 digests relative to that directory. Stage a new root, atomically rename the manifest into
// place, and retain the old root until in-flight fetches finish.
//
// Example usage:
//
//	source, err := filesystem.New("/path/to/policies",
//	    filesystem.WithReleaseManifest("release.json"),
//	    filesystem.WithIncludeData(),
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
