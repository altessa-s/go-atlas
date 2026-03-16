// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlss3

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options --all-fields

import (
	"context"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/altessa-s/go-atlas/security/tlsutils"
)

// SSEType identifies the server-side encryption type for S3 objects.
type SSEType string

const (
	// SSETypeNone indicates no server-side encryption.
	SSETypeNone SSEType = ""
	// SSETypeS3 indicates SSE-S3 (AES-256) encryption managed by S3.
	SSETypeS3 SSEType = "s3"
	// SSETypeKMS indicates SSE-KMS encryption using AWS KMS keys.
	SSETypeKMS SSEType = "kms"
	// SSETypeC indicates SSE-C encryption using customer-provided keys.
	SSETypeC SSEType = "c"
)

// S3API is the subset of the S3 client API used by this provider.
// *s3.Client satisfies this interface.
type S3API interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
}

type options struct {
	logger            *slog.Logger
	ocspStapler       tlsutils.OCSPStapler `optgen:"notnil"`
	ctx               context.Context      `opt:"Context"`
	pollInterval      time.Duration
	reloadNotifyChan  chan struct{}
	sseType           SSEType
	sseKMSKeyID       string
	sseCustomerKey    string
	sseCustomerKeyMD5 string
	s3Client          S3API `optgen:"notnil"`
}
