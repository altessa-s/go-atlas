// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package kms provides key material and optional TLS configuration for MongoDB CSFLE.
// Implementations are in subpackages: aws, azure, gcp, local.
//
// Example:
//
//	var p kms.MasterKeyProvider = kmsaws.New(accessKey, secretKey, keyArn)
//	creds := p.Credentials()
//	defer p.Clear() // securely clear credentials
package kms
