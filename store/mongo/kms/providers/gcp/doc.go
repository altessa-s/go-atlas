// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package kmsgcp provides Google Cloud KMS integration for MongoDB CSFLE.
// Credentials are stored securely and automatically cleared when no longer needed.
//
// Example:
//
//	p := kmsgcp.New(email, privateKey, projectId, location, keyRing, keyName)
//	defer p.Clear()
package kmsgcp
