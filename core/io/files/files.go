// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package files

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// FileExists reports whether name refers to an existing regular file (not a directory).
// It returns false if the path does not exist, is a directory, or cannot be stat'd.
//
// Example:
//
//	files.FileExists("/path/to/file.txt") // true or false
func FileExists(name string) bool {
	info, err := os.Stat(name)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// DirExists reports whether name refers to an existing directory. It returns false
// if the path does not exist, is a regular file, or cannot be stat'd.
//
// Example:
//
//	files.DirExists("/path/to/directory") // true or false
func DirExists(name string) bool {
	i, err := os.Stat(name)
	if err == nil && i.IsDir() {
		return true
	}

	return false
}

// DirIsEmpty reports whether the directory at name contains no entries. It returns
// (true, nil) for an empty directory, (false, nil) when at least one entry exists,
// and (false, err) if the directory cannot be opened or read. The directory is opened
// read-only and closed before returning.
//
// Example:
//
//	isEmpty, err := files.DirIsEmpty("/path/to/directory")
//
// #nosec G304 -- path comes from trusted application code
func DirIsEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = f.Close() //nolint: errcheck
	}()

	_, err = f.ReadDir(1)
	if errors.Is(err, io.EOF) {
		return true, nil
	}
	return false, err // Either not empty or error, suits both cases
}

// FindFile searches for a file named filePath within the directories listed in
// baseSearchPaths. If filePath is absolute, it is checked directly and returned
// unchanged when it exists; otherwise the empty string is returned. For relative paths,
// each entry in baseSearchPaths is resolved relative to the directory of the current
// executable (see [CurrentExecutableDir]); entries that are already absolute are used
// as-is. The first path that resolves to an existing regular file is returned as an
// absolute path. If no match is found, the empty string is returned.
//
// Example:
//
//	path := files.FindFile("config.yaml", []string{"config", "/etc/app"})
func FindFile(filePath string, baseSearchPaths []string) string {
	if filepath.IsAbs(filePath) {
		_, err := os.Stat(filePath)
		if err == nil {
			return filePath
		}
		return ""
	}

	var searchPaths []string
	if dir := CurrentExecutableDir(); dir != "" {
		for _, baseSearchPath := range baseSearchPaths {
			if !filepath.IsAbs(baseSearchPath) {
				searchPaths = append(
					searchPaths,
					filepath.Join(dir, baseSearchPath),
				)
				continue
			}
			searchPaths = append(searchPaths, baseSearchPath)
		}
	}
	for _, parent := range searchPaths {
		found, err := filepath.Abs(filepath.Join(parent, filePath))
		if err != nil {
			continue
		}
		fileInfo, err := os.Stat(found)
		if err == nil && !fileInfo.IsDir() {
			return found
		}
	}
	return ""
}

// CurrentExecutableDir returns the absolute path of the directory containing the
// currently running executable, with symlinks resolved via [filepath.EvalSymlinks].
// If any step (locating the executable, resolving symlinks, or computing the
// absolute path) fails, the empty string is returned.
//
// Example:
//
//	exeDir := files.CurrentExecutableDir()
func CurrentExecutableDir() string {
	if exe, err := os.Executable(); err == nil {
		if exe, err = filepath.EvalSymlinks(exe); err == nil {
			if exe, err = filepath.Abs(exe); err == nil {
				return filepath.Dir(exe)
			}
		}
	}

	return ""
}
