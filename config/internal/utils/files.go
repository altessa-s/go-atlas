// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package utils

import (
	"os"
	"path/filepath"
)

// FindFile attempts to locate a file by checking if the provided path exists.
// It only validates absolute paths and returns the path if the file exists,
// otherwise returns an empty string.
//
// Example:
//
//	path := utils.FindFile("/etc/ssl/certs/ca.pem")
func FindFile(filePath string) string {
	if filePath == "" {
		return ""
	}

	if filepath.IsAbs(filePath) {
		_, err := os.Stat(filePath)
		if err == nil {
			return filePath
		}
	}

	return ""
}
