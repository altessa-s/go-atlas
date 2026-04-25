// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package files provides utilities for file system operations.
// Includes file/directory existence checks, emptiness verification, multi-path
// search, executable path resolution, and an iterator-based directory walker
// with optional extension and file-type filtering.
//
// Example:
//
//	if files.FileExists("/path/to/file.txt") {
//	    fmt.Println("File exists")
//	}
//	configPath := files.FindFile("config.yaml", []string{"config", "/etc/app"})
//
//	for entry, err := range files.Walk("./plugins",
//	    files.WithExtensions(".so"),
//	    files.WithFileTypes(files.FileTypeRegular),
//	) {
//	    if err != nil {
//	        return err
//	    }
//	    fmt.Println(entry.Path)
//	}
package files
