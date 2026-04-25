// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package s3

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import (
	"context"
	"log/slog"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3API is the subset of the S3 client API used by this source.
// *s3.Client satisfies this interface.
type S3API interface {
	ListObjectsV2(ctx context.Context, params *awss3.ListObjectsV2Input, optFns ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error)
	GetObject(ctx context.Context, params *awss3.GetObjectInput, optFns ...func(*awss3.Options)) (*awss3.GetObjectOutput, error)
}

// DefaultExtensions are the default file extensions to include.
var DefaultExtensions = []string{".rego"}

// options holds the configuration for the S3 source.
type options struct {
	// s3Client is the S3 API client used to list and download objects.
	s3Client S3API `optgen:"notnil"`
	// bucket is the S3 bucket containing policy files.
	bucket string
	// prefix is the object key prefix (directory-like path) within the bucket.
	prefix string
	// extensions sets the file extensions to include when loading policies.
	// Defaults to [".rego"].
	extensions []string `optgen:"default=DefaultExtensions"`
	// includeData enables loading .json files as OPA data.
	includeData bool
	// logger sets the logger for the S3 source.
	logger *slog.Logger
}
