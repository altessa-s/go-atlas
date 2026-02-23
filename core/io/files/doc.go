// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package files provides utilities for file system operations.
// Includes file/directory existence checks, emptiness verification, multi-path search, and executable path resolution.
//
// Example:
//
//	if files.FileExists("/path/to/file.txt") {
//	    fmt.Println("File exists")
//	}
//	configPath := files.FindFile("config.yaml", []string{"config", "/etc/app"})
package files
