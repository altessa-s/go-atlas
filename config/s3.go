// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"github.com/go-ozzo/ozzo-validation/is"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for S3 configuration.
const (
	defaultS3Endpoint = "http://localhost:9000"
)

// S3 represents the configuration for Amazon S3 or S3-compatible storage services.
// It contains the necessary authentication and connection details for accessing
// S3-compatible object storage.
//
// Example:
//
//	s3 := &config.S3{
//		Endpoint:  "https://s3.amazonaws.com",
//		AccessKey: "$__secret{s3:accessKey}",
//		SecretKey: "$__secret{s3:secretKey}",
//	}
type S3 struct {
	// Endpoint is the S3 service endpoint URL.
	// Defaults to "http://localhost:9000" for local MinIO development.
	// For AWS S3, use regional endpoints like "https://s3.us-west-2.amazonaws.com".
	Endpoint string `yaml:"endpoint" default:"http://localhost:9000"`

	// AccessKey is the access key ID for S3 authentication.
	// This is required and should be kept secure.
	AccessKey Secret `yaml:"accessKey"`

	// SecretKey is the secret access key for S3 authentication.
	// This is required and should be kept secure.
	SecretKey Secret `yaml:"secretKey"`

	// PathStyle enables path-style addressing instead of virtual-hosted-style.
	// Use for MinIO or S3-compatible services that do not support virtual-hosted-style URLs.
	PathStyle bool `yaml:"pathStyle"`

	// Region is the AWS region for the S3 bucket (e.g. "us-east-1", "eu-west-1").
	// This is required for proper request signing and endpoint resolution.
	Region string `yaml:"region"`
}

// DefaultS3 returns an S3 configuration with default values.
// Note: AccessKey and SecretKey are left as zero values since they are required fields.
func DefaultS3() S3 {
	return S3{
		Endpoint: defaultS3Endpoint,
	}
}

// Validate performs validation on the S3 configuration.
// It ensures the endpoint is a valid URL and that both access key
// and secret key are provided.
//
// Returns an error if any validation rules fail.
func (m *S3) Validate() error {
	return ValidateStruct(m,
		validation.Field(&m.Endpoint, validation.Required, is.URL),
		validation.Field(&m.AccessKey, validation.Required),
		validation.Field(&m.SecretKey, validation.Required),
		validation.Field(&m.Region, validation.Required),
	)
}
