// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// SSETypeConfig identifies the server-side encryption type for S3 objects in configuration.
type SSETypeConfig string

const (
	// SSETypeConfigS3 indicates SSE-S3 (AES-256) encryption managed by S3.
	SSETypeConfigS3 SSETypeConfig = "s3"

	// SSETypeConfigKMS indicates SSE-KMS encryption using AWS KMS keys.
	SSETypeConfigKMS SSETypeConfig = "kms"

	// SSETypeConfigC indicates SSE-C encryption using customer-provided keys.
	SSETypeConfigC SSETypeConfig = "c"
)

// TlsProviderS3 configures S3-based TLS certificate provider.
// It downloads certificates and private keys from S3-compatible storage
// (AWS S3, MinIO) with support for all SSE encryption types and periodic
// polling for changes.
type TlsProviderS3 struct {
	// S3 contains the S3 connection configuration (endpoint, credentials, region).
	S3 `yaml:",inline"`

	// Bucket is the S3 bucket containing the certificate files.
	Bucket string `yaml:"bucket"`

	// CertificateKey is the S3 object key for the PEM-encoded certificate.
	CertificateKey string `yaml:"certificateKey"`

	// PrivateKeyKey is the S3 object key for the PEM-encoded private key.
	PrivateKeyKey string `yaml:"privateKeyKey"`

	// PrivateKeyPassword for encrypted private keys (optional).
	PrivateKeyPassword Secret `yaml:"privateKeyPassword"`

	// PollInterval specifies how often to check S3 for certificate changes.
	// Defaults to 5 minutes.
	PollInterval time.Duration `yaml:"pollInterval" default:"5m"`

	// SSE configures server-side encryption for reading certificate objects.
	SSE *TlsProviderS3SSE `yaml:"sse" default:"-"`
}

// TlsProviderS3SSE configures server-side encryption for S3 certificate objects.
type TlsProviderS3SSE struct {
	// Type specifies the SSE encryption type: "s3", "kms", or "c".
	Type SSETypeConfig `yaml:"type"`

	// KMSKeyID is the AWS KMS key ARN or ID. Required when Type is "kms".
	KMSKeyID string `yaml:"kmsKeyId"`

	// CustomerKey is the customer-provided encryption key. Required when Type is "c".
	CustomerKey Secret `yaml:"customerKey"`

	// CustomerKeyMD5 is the base64-encoded MD5 of the customer key. Required when Type is "c".
	CustomerKeyMD5 string `yaml:"customerKeyMd5"`
}

// Validate checks that the S3 provider configuration is valid.
func (c *TlsProviderS3) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.S3),
		validation.Field(&c.Bucket, validation.Required),
		validation.Field(&c.CertificateKey, validation.Required),
		validation.Field(&c.PrivateKeyKey, validation.Required),
		validation.Field(&c.PollInterval, ozzo_rules.DurationOrZero()),
		validation.Field(&c.SSE, validation.Required.When(c.SSE != nil)),
	)
}

// Validate checks that the SSE configuration is valid.
func (c *TlsProviderS3SSE) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Type, validation.Required, ozzo_rules.OneOf(SSETypeConfigS3, SSETypeConfigKMS, SSETypeConfigC)),
		validation.Field(&c.KMSKeyID, validation.Required.When(c.Type == SSETypeConfigKMS)),
		validation.Field(&c.CustomerKey, validation.Required.When(c.Type == SSETypeConfigC)),
		validation.Field(&c.CustomerKeyMD5, validation.Required.When(c.Type == SSETypeConfigC)),
	)
}
