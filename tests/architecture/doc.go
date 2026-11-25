// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package architecture checks production dependency boundaries across platforms.
//
// It checks runtime imports of test fixtures and verifies that production imports
// exercise each declared dependency between planned modules. Public transport signatures
// and an external server consumer are checked for internal type leaks.
//
// # Usage
//
//	go test ./tests/architecture
package architecture
