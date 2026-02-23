// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsaws

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"crypto/tls"
)

// options contains AWS KMS provider configuration.
type options struct {
	// Region sets the AWS region for the KMS service.
	// This option allows overriding the default region.
	// Example: "us-east-1", "eu-west-1", "ap-southeast-1"
	region *string

	// Endpoint sets a custom AWS KMS endpoint URL.
	// This is typically used for testing or when using AWS-compatible services.
	// Example: "https://kms.us-east-1.amazonaws.com" or "http://localhost:4566" for LocalStack
	endpoint *string

	// TLS sets a custom TLS configuration for connections to AWS KMS.
	// This allows fine-grained control over TLS settings for secure connections.
	tLS *tls.Config
}
