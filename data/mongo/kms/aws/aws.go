// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsaws

import (
	"crypto/tls"
	"sync/atomic"

	"github.com/altessa-s/go-atlas/data/mongo/kms"
)

// Amazon represents an AWS KMS provider configuration.
// It implements the kms.Provider, kms.MasterKey, and kms.TLSConfig interfaces.
// Credentials are stored securely using the credentials package.
type Amazon struct {
	credentialsCache atomic.Pointer[kms.Credentials]
	// credentials stores sensitive AWS authentication data securely
	credentials *AWSCredentials
	// Key is the AWS KMS key ARN or alias
	Key string
	// Region is the optional AWS region (e.g., "us-east-1")
	Region *string
	// Endpoint is the optional custom AWS KMS endpoint URL
	Endpoint *string
	// TLS is the optional custom TLS configuration
	TLS *tls.Config
}

// New creates a new AWS KMS provider with the specified credentials and key.
// Credentials are stored securely and automatically cleared when no longer needed.
//
// Parameters:
//   - accessKeyID: AWS access key ID for authentication (will be stored securely)
//   - secretAccessKey: AWS secret access key for authentication (will be stored securely)
//   - keyName: AWS KMS key ARN or alias (e.g., "arn:aws:kms:us-east-1:123456789:key/12345678-1234-1234-1234-123456789012")
//   - opts: Optional configuration options
//
// Example:
//
//	aws := kmsaws.New("AKIA...", "secretkey", "arn:aws:kms:us-east-1:123:key/abc",
//		kmsaws.WithRegion("us-east-1"),
//		kmsaws.WithSessionToken("token"))
func New(accessKeyID, secretAccessKey, keyName string, opts ...Option) *Amazon {
	options := newOptions(opts...)

	aws := &Amazon{
		credentials: NewAWSCredentials(accessKeyID, secretAccessKey, nil),
		Key:         keyName,
		Region:      options.region,
		Endpoint:    options.endpoint,
		TLS:         options.tLS,
	}

	return aws
}

// Name returns the provider name "aws".
// This implements the kms.Provider interface.
func (a *Amazon) Name() string {
	return ProviderName
}

// Credentials returns the AWS authentication credentials.
// This implements the kms.Provider interface.
// Note: This method exposes sensitive credentials - use with caution.
func (a *Amazon) Credentials() kms.Credentials {
	if cred := a.credentialsCache.Load(); cred != nil {
		return *cred
	}

	if a.credentials == nil {
		return kms.Credentials{a.Name(): {}}
	}

	cred := kms.Credentials{
		a.Name(): {
			AccessKeyID:     a.credentials.AccessKeyID.StringUnsafe(),
			SecretAccessKey: a.credentials.SecretAccessKey.StringUnsafe(),
		},
	}

	if a.credentials.SessionToken != nil && !a.credentials.SessionToken.IsEmpty() {
		cred[a.Name()][SessionToken] = a.credentials.SessionToken.StringUnsafe()
	}

	a.credentialsCache.Store(&cred)

	return cred
}

// MasterKey returns the AWS KMS master key configuration.
// This implements the kms.MasterKey interface.
func (a *Amazon) MasterKey() kms.Key {
	key := kms.Key{KeyARN: a.Key}

	if a.Region != nil {
		key[Region] = *a.Region
	}

	if a.Endpoint != nil {
		key[Endpoint] = *a.Endpoint
	}

	return key
}

// TLSConfig returns the TLS configuration for AWS KMS connections.
// This implements the simplified kms.Provider interface.
func (a *Amazon) TLSConfig() *tls.Config {
	return a.TLS
}

// Clear securely clears all stored credentials and sensitive data.
// This method should be called when the AWS KMS provider is no longer needed
// to ensure sensitive credentials don't remain in memory.
func (a *Amazon) Clear() {
	if a.credentials != nil {
		a.credentials.Clear()
		a.credentials = nil
	}
}
