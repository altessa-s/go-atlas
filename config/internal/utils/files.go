// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package utils

import "path/filepath"

// ResolvePath normalizes a configured file path. An absolute path is returned
// unchanged and a relative path is resolved against the current working
// directory, so the result keeps pointing at the same file if the process
// changes directory later. An empty path stays empty.
//
// ResolvePath never checks that the file exists and never drops a path: a
// missing file keeps its (resolved) path so the schema's Validate reports it
// instead of the path silently becoming "". When the working directory cannot
// be determined, a relative path is returned as written.
//
// Example:
//
//	path := utils.ResolvePath("certs/ca.pem") // "/srv/app/certs/ca.pem" when run from /srv/app
func ResolvePath(filePath string) string {
	if filePath == "" || filepath.IsAbs(filePath) {
		return filePath
	}

	abs, err := filepath.Abs(filePath)
	if err != nil {
		return filePath
	}

	return abs
}
