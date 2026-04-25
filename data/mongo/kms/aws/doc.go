// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package kmsaws provides AWS KMS integration for MongoDB CSFLE.
// Credentials are stored securely and automatically cleared when no longer needed.
//
// Example:
//
//	p := kmsaws.New("AKIA...", "secret", "arn:aws:kms:...",
//	    kmsaws.WithRegion("us-east-1"))
//	defer p.Clear()
package kmsaws
