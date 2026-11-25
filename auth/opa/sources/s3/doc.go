// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package s3 implements [opa.PolicySource] backed by an S3-compatible
// object store (AWS S3, MinIO, etc.). It lists and downloads .rego
// policy files from a bucket/prefix. Change detection is handled by
// the [opa.Manager].
// JSON data files preserve number precision.
// A nonempty prefix selects a directory. Both policies and policies/ list objects below policies/.
//
// # Release manifests
//
// Each fetch checks the SHA-256 digest and uses a conditional GetObject for
// every included object. Use WithReleaseManifest for a release manifest
// object or WithChecksums for a fixed release.
// For rollouts, the manifest's root selects a release prefix and its files map lists ETags and
// SHA-256 digests relative to that prefix. Upload the new release first, replace the manifest
// with one PUT, and retain the old release until in-flight fetches finish.
//
// # Usage
//
//	source, err := s3.New(
//	    s3.WithS3Client(s3Client),
//	    s3.WithBucket("my-policies"),
//	    s3.WithPrefix("opa/"),
//	    s3.WithReleaseManifest("release.json"),
//	)
//	if err != nil {
//	    return err
//	}
//	defer source.Close()
//
//	bundle, err := source.Fetch(ctx)
package s3
