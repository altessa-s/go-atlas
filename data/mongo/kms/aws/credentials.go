// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsaws

import (
	"github.com/altessa-s/go-atlas/core/text/strings"
)

// AWSCredentials provides secure storage for AWS KMS credentials.
type AWSCredentials struct {
	AccessKeyID     *strings.SecureString
	SecretAccessKey *strings.SecureString
	SessionToken    *strings.SecureString
}

// NewAWSCredentials creates a new AWSCredentials instance with secure storage.
//
// Parameters:
//   - accessKeyID: AWS access key ID (will be stored securely)
//   - secretAccessKey: AWS secret access key (will be stored securely)
//   - sessionToken: Optional AWS session token (will be stored securely)
//
// Returns:
//   - *AWSCredentials: New AWS credentials instance with secure storage
func NewAWSCredentials(accessKeyID, secretAccessKey string, sessionToken *string) *AWSCredentials {
	creds := &AWSCredentials{
		AccessKeyID:     strings.NewSecureString(accessKeyID),
		SecretAccessKey: strings.NewSecureString(secretAccessKey),
	}

	if sessionToken != nil {
		creds.SessionToken = strings.NewSecureString(*sessionToken)
	}

	return creds
}

// Clear clears all stored credentials and zeros sensitive memory.
func (c *AWSCredentials) Clear() {
	if c.AccessKeyID != nil {
		c.AccessKeyID.Clear()
		c.AccessKeyID = nil
	}

	if c.SecretAccessKey != nil {
		c.SecretAccessKey.Clear()
		c.SecretAccessKey = nil
	}

	if c.SessionToken != nil {
		c.SessionToken.Clear()
		c.SessionToken = nil
	}
}
