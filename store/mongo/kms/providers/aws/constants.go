// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsaws

// ProviderName is the identifier for AWS KMS provider
const ProviderName = "aws"

const (
	// KeyARN is the master key field name for AWS KMS key ARN
	KeyARN = "key"

	// Region is the master key field name for AWS region
	Region = "region"

	// Endpoint is the master key field name for AWS KMS endpoint (optional)
	Endpoint = "endpoint"
)

const (
	// AccessKeyID is the credential field name for AWS access key ID
	AccessKeyID = "accessKeyId"

	// SecretAccessKey is the credential field name for AWS secret access key
	SecretAccessKey = "secretAccessKey"

	// SessionToken is the credential field name for AWS session token (optional)
	SessionToken = "sessionToken"
)
