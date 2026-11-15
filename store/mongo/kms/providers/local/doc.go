// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package kmslocal provides local key management for MongoDB CSFLE.
// The master key must be exactly 96 bytes as required by MongoDB CSFLE.
//
// Example:
//
//	p, err := kmslocal.New(kmslocal.WithMasterKeyFile("/path/to/key"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer p.Clear()
package kmslocal
