// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package s3 implements [opa.PolicySource] backed by an S3-compatible
// object store (AWS S3, MinIO, etc.). It lists and downloads .rego
// policy files from a bucket/prefix. Change detection is handled by
// the [opa.Manager].
//
// # Usage
//
//	source, err := s3.New(
//	    s3.WithS3Client(s3Client),
//	    s3.WithBucket("my-policies"),
//	    s3.WithPrefix("opa/"),
//	)
//	if err != nil {
//	    return err
//	}
//	defer source.Close()
//
//	bundle, err := source.Fetch(ctx)
package s3
