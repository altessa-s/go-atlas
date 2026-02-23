// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package id

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/core/runtime/panics"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// File is a [Provider] that persists the Service ID to a file on disk.
//
// On the first call to [File.ID], the provider reads the first line of the
// configured file. If the file does not exist or is empty, a new 128-bit
// random hex ID is generated and written back for future process restarts.
//
// All methods are safe for concurrent use; initialization is guarded by a
// [sync.Once]. Two categories of errors are tracked separately:
//
//   - A fatal initialization error (e.g. unreadable file) causes [File.ID] to
//     return an empty string. Retrieve it with [File.Error].
//   - A non-fatal persistence error (e.g. unable to write a newly generated ID)
//     still allows [File.ID] to return the in-memory value. Retrieve it with
//     [File.PersistenceError].
type File struct {
	id         string    // The cached Service ID
	filePath   string    // Path to the file containing the ID
	once       sync.Once // Ensures ID is initialized only once
	initErr    error     // Stores any initialization error (fatal - prevents ID from being returned)
	persistErr error     // Stores any persistence error (non-fatal - ID still returned but not persisted)
}

// NewFile creates a new [File] provider bound to the given file path.
// The file is not accessed until the first call to [File.ID], so construction
// succeeds even when the file does not yet exist.
//
// Returns an error if filePath is the empty string.
func NewFile(filePath string) (*File, error) {
	if filePath == "" {
		return nil, fmt.Errorf("id: file path cannot be empty")
	}

	s := &File{
		filePath: filePath,
	}
	return s, nil
}

// MustNewFile is like [NewFile] but panics if the provider cannot be created.
// It is intended for program-level initialization where failure is
// unrecoverable.
func MustNewFile(filePath string) *File {
	return panics.MustResult(NewFile(filePath))
}

// ID returns the Service ID, lazily initializing it on the first call.
//
// Initialization reads the existing file, or generates a new 128-bit random
// hex string and persists it. The result is cached so subsequent calls return
// the same value without I/O. Concurrent callers block until the first
// initialization completes.
//
// Returns an empty string if a fatal initialization error occurred; call
// [File.Error] to inspect the cause.
func (s *File) ID() string {
	s.once.Do(func() {
		s.initializeID()
	})
	if s.initErr != nil {
		// Return empty string on error instead of panicking
		// Callers can check for empty ID or use Error() method
		return ""
	}
	return s.id
}

// Error returns the fatal initialization error, if any, that prevents
// [File.ID] from returning a valid value. It triggers lazy initialization
// if it has not already occurred.
//
// Returns nil when initialization succeeded. For non-fatal write failures,
// use [File.PersistenceError] instead.
func (s *File) Error() error {
	// Ensure initialization has been attempted
	s.once.Do(func() {
		s.initializeID()
	})
	return s.initErr
}

// PersistenceError returns the non-fatal error that occurred when writing a
// newly generated ID back to disk. When this error is non-nil, [File.ID]
// still returns a valid in-memory value for the lifetime of the current
// process, but a new ID will be generated on the next restart. It triggers
// lazy initialization if it has not already occurred.
//
// Returns nil when the ID was read from an existing file or when the write
// succeeded.
func (s *File) PersistenceError() error {
	// Ensure initialization has been attempted
	s.once.Do(func() {
		s.initializeID()
	})
	return s.persistErr
}

// initializeID performs the one-time initialization of the ID.
// This method is called exactly once by sync.Once, ensuring thread safety.
func (s *File) initializeID() {
	var id string

	// First, try to read from existing file
	if fileExists(s.filePath) {
		existingID, err := readLine(s.filePath)
		if err != nil {
			// If the file exists but can't be read, fail fast to avoid silently changing service identity.
			s.initErr = coreerrs.WrapOperation(err, "read ID file "+s.filePath)
			return
		}
		if trimmed := strings.TrimSpace(existingID); trimmed != "" {
			id = trimmed
		}
	}

	// If no valid ID found in file, generate a new one
	if id == "" {
		// Generate 16 random bytes (128 bits of entropy)
		// This is sufficient for a unique Service ID
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			// Store error instead of panicking
			s.initErr = coreerrs.WrapOperation(err, "generate random ID")
			return
		}
		id = strings.ToUpper(hex.EncodeToString(b))

		// Write the new ID to the file for future use.
		// If this fails, we still use the ID for this process but track the error.
		// Callers should check PersistenceError() to detect this condition.
		if err := writeLine(s.filePath, id); err != nil {
			s.persistErr = coreerrs.WrapOperation(err, "persist ID to "+s.filePath)
		}
	}

	// Cache the ID value (no mutex needed since sync.Once guarantees single execution)
	s.id = id
}

// writeLine writes a string to a file.
// It creates the file if it doesn't exist, or truncates it if it does.
// #nosec G304 -- filepath comes from trusted configuration
func writeLine(filepath string, id string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return coreerrs.WrapOperation(err, "create file")
	}
	defer func() { _ = file.Close() }()

	_, err = file.Write([]byte(id))
	if err != nil {
		return coreerrs.WrapOperation(err, "write to file")
	}
	return nil
}

// readLine reads the first line from a file.
// It returns an empty string and an error if the file doesn't exist or can't be read.
// #nosec G304 -- filepath comes from trusted configuration
func readLine(filepath string) (string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return "", coreerrs.WrapOperation(err, "open file")
	}
	defer func() { _ = file.Close() }()

	fileScanner := bufio.NewScanner(file)

	var id string
	if fileScanner.Scan() {
		id = fileScanner.Text()
	}

	// Always check for scanner errors after iteration completes.
	// Scan() returns false on EOF (normal) or on error - we must check which.
	if err := fileScanner.Err(); err != nil {
		return "", coreerrs.WrapOperation(err, "scan file")
	}

	return id, nil
}

// fileExists checks if a file exists and is accessible.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}

	return true
}

// Runtime check to ensure that the File type implements the Provider interface.
var _ Provider = (*File)(nil)
