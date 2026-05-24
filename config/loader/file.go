// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader

import (
	"bufio"
	"fmt"
	"hash/crc32"
	"io"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/config/loader/backend"

	coreio "github.com/altessa-s/go-atlas/core/io"
)

const (
	// File seek position constants
	seekStart = 0
)

// files manages a collection of configuration files with thread-safe access.
type files struct {
	files map[string]*file
	mx    sync.RWMutex
}

// file represents a single configuration file with its metadata and decoder.
type file struct {
	decoder   backend.Decoder
	name      string
	path      string
	sum       string
	isSymlink bool
}

// loadAndDecode loads the file and decodes it into the provided interface.
// It opens the file, decodes its content, and calculates a checksum for change detection.
func (cf *Config) loadAndDecode(f *file, out any) (err error) {
	var osFile *os.File

	osFile, err = os.OpenFile(f.path, os.O_RDONLY, os.ModeType)
	if err != nil {
		err = fmt.Errorf("%w: %s: %w", ErrDecode, f.name, err)
		return
	}

	defer func() {
		_ = osFile.Close()
	}()

	// Read file content for environment variable substitution. The read
	// is bounded by maxConfigBytes (default DefaultMaxConfigBytes, 16
	// MiB): without a cap, a symlink to /dev/zero or a multi-GB tmpfs
	// file would OOM the loader before the YAML parser ever rejected
	// the input. Operators with genuinely huge configs can raise the
	// cap via WithMaxConfigBytes.
	maxBytes := cf.options.maxConfigBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxConfigBytes
	}
	limited := coreio.NewLimitedReadCloser(osFile, maxBytes)
	content, err := io.ReadAll(limited)
	if err != nil && err != io.EOF {
		err = fmt.Errorf("%w: %s: %w", ErrDecode, f.name, err)
		return
	}
	// Clear error if it was just EOF (empty file is OK)
	if err == io.EOF {
		err = nil
	}

	// Perform pre-processing if supported by the backend
	processedContent := string(content)
	if preprocessor, ok := f.decoder.(backend.Preprocessor); ok {
		// Determine root directory for security checks (base path of the initial config)
		var rootDir string
		if cf.options.path != "" {
			if fixedPath, fixErr := fixPath(cf.options.path); fixErr == nil {
				fi, statErr := os.Stat(fixedPath)
				if statErr == nil && !fi.IsDir() {
					rootDir = filepath.Dir(fixedPath)
				} else {
					rootDir = fixedPath
				}
			}
		}

		if rootDir == "" {
			rootDir = filepath.Dir(f.path)
		}

		processedContent, err = preprocessor.Preprocess(string(content), filepath.Dir(f.path), rootDir)
		if err != nil {
			err = fmt.Errorf("%w: %s: %w", ErrDecode, f.name, err)
			return
		}
	}

	// Perform environment variable substitution on raw text
	var substitutedContent string
	if cf.options.strict {
		substitutedContent, err = substituteEnvVariablesStrict(processedContent)
		if err != nil {
			err = fmt.Errorf("%w: %s: %w", ErrDecode, f.name, err)
			return
		}
	} else {
		substitutedContent = substituteEnvVariables(processedContent)
	}

	// Handle empty content or comments-only content - provide a minimal valid YAML document
	if strings.TrimSpace(substitutedContent) == "" || isEmptyOrCommentsOnly(substitutedContent) {
		substitutedContent = "{}\n"
	}

	// Create a reader from the substituted content
	substitutedReader := strings.NewReader(substitutedContent)

	err = f.decoder.Decode(substitutedReader, out)
	if err != nil {
		err = fmt.Errorf("%w: %s: %w", ErrDecode, f.name, err)
		return
	}

	f.sum, err = f.calculateSum(osFile)

	return
}

// calculateSum calculates the CRC32 checksum of the file.
// If osFile is nil, it opens the file first. Otherwise, it uses the provided file handle.
func (f *file) calculateSum(osFile *os.File) (string, error) {
	if osFile == nil {
		var err error
		if osFile, err = os.OpenFile(f.path, os.O_RDONLY, os.ModeType); err != nil {
			return "", err
		}

		defer func() {
			_ = osFile.Close()
		}()
	}

	if _, err := osFile.Seek(seekStart, seekStart); err != nil {
		return "", err
	}

	h := crc32.NewIEEE()
	if _, err := io.Copy(h, bufio.NewReader(osFile)); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum32()), nil
}

// newFiles creates a new files instance with an initialized map.
func newFiles() *files {
	return &files{
		files: make(map[string]*file),
	}
}

// add adds a new file to the internal map if it doesn't already exist.
// This method is thread-safe.
func (fs *files) add(f *file) {
	fs.mx.Lock()
	defer fs.mx.Unlock()

	if _, ok := fs.files[f.name]; !ok {
		fs.files[f.name] = f
	}
}

// All returns an iterator for the files collection.
// It ensures thread-safe access during iteration by holding a read lock.
func (fs *files) All() iter.Seq[*file] {
	return func(yield func(*file) bool) {
		fs.mx.RLock()
		ordered := make([]*file, 0, len(fs.files))
		for _, f := range fs.files {
			ordered = append(ordered, f)
		}
		fs.mx.RUnlock()

		slices.SortFunc(ordered, func(a, b *file) int {
			if a.name != b.name {
				if a.name < b.name {
					return -1
				}
				return 1
			}
			if a.path < b.path {
				return -1
			}
			if a.path > b.path {
				return 1
			}
			return 0
		})

		for _, f := range ordered {
			if !yield(f) {
				return
			}
		}
	}
}

// isEmptyOrCommentsOnly checks if the content contains only comments and whitespace.
// Returns true if the content has no actual YAML configuration data.
func isEmptyOrCommentsOnly(content string) bool {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip empty lines and comment lines
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return false // Found non-comment content
		}
	}
	return true // Only comments and empty lines found
}
