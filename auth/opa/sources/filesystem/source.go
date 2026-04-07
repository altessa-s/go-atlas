// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filesystem

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/core/io/files"

	corehash "github.com/altessa-s/go-atlas/core/encoding/hash"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

var (
	// ErrChecksumMismatch is returned when a policy file's SHA-256 hash does not match the expected value.
	ErrChecksumMismatch = errors.New("policy file checksum mismatch")
	// ErrUnexpectedPolicyFile is returned when a policy file is found on disk but has no entry in the checksums manifest.
	ErrUnexpectedPolicyFile = errors.New("policy file not in checksums manifest")
	// ErrMissingPolicyFile is returned when an entry in the checksums manifest has no corresponding file on disk.
	ErrMissingPolicyFile = errors.New("expected policy file not found")
)

// Source implements opa.PolicySource for filesystem-based policies.
// It reads .rego files from a directory. Change detection is handled by the Manager.
type Source struct {
	path       string
	opts       *options
	logger     *slog.Logger
	extensions map[string]struct{}

	mu     sync.Mutex
	closed bool
}

// New creates a new filesystem policy source.
// The path can be a directory containing .rego files or a single .rego file.
func New(path string, opts ...Option) (*Source, error) {
	isDir := files.DirExists(path)
	isFile := files.FileExists(path)

	if !isDir && !isFile {
		return nil, fmt.Errorf("invalid path %q: path does not exist", path)
	}

	if !isDir && !strings.HasSuffix(path, ".rego") {
		return nil, fmt.Errorf("path must be a directory or .rego file: %s", path)
	}

	o := newOptions(opts...)

	extensions := make(map[string]struct{}, len(o.extensions))
	for _, ext := range o.extensions {
		extensions[ext] = struct{}{}
	}

	return &Source{
		path:       path,
		opts:       o,
		logger:     cmp.Or(o.logger, slog.New(slog.DiscardHandler)),
		extensions: extensions,
	}, nil
}

// Name returns the source identifier.
func (s *Source) Name() string {
	return "filesystem:" + s.path
}

// Fetch reads all policy files from the configured path.
func (s *Source) Fetch(ctx context.Context) (*opa.PolicyBundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, opa.ErrSourceClosed
	}

	modules := make(map[string][]byte)
	var data map[string]any

	info, err := os.Stat(s.path)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "stat path")
	}

	if info.IsDir() {
		if err := s.walkDirectory(s.path, modules, &data); err != nil {
			return nil, err
		}
	} else {
		content, err := os.ReadFile(s.path)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "read policy file")
		}

		relPath := relativePolicyPath(filepath.Dir(s.path), s.path)
		if err := s.verifyChecksum(relPath, content); err != nil {
			return nil, err
		}

		modules[s.path] = content
	}

	// Build a relPath→true map for the coverage check.
	loadedRelPaths := make(map[string]string, len(modules))
	for absPath := range modules {
		rel := relativePolicyPath(s.path, absPath)
		if !info.IsDir() {
			rel = relativePolicyPath(filepath.Dir(s.path), absPath)
		}
		loadedRelPaths[rel] = absPath
	}

	if err := s.verifyAllChecksumsCovered(loadedRelPaths); err != nil {
		return nil, err
	}

	if len(modules) == 0 {
		return nil, coreerrs.Wrapf(opa.ErrNoPolicyFiles, "%s", s.path)
	}

	var bundle *opa.PolicyBundle
	if data != nil {
		bundle = opa.NewPolicyBundleWithData(modules, data)
	} else {
		bundle = opa.NewPolicyBundle(modules)
	}

	s.logger.Debug("fetched policy bundle",
		slog.String("path", s.path),
		slog.Int("modules", len(modules)),
		slog.String("revision", bundle.Revision))

	return bundle, nil
}

// walkDirectory recursively walks the directory and collects policy files.
func (s *Source) walkDirectory(root string, modules map[string][]byte, data *map[string]any) error {
	for entry, err := range files.Walk(root,
		files.WithRecursive(),
		files.WithFileTypes(files.FileTypeRegular),
	) {
		if err != nil {
			return err
		}

		path := entry.Path
		ext := filepath.Ext(entry.Name())

		// Check for policy files
		if _, ok := s.extensions[ext]; ok {
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return coreerrs.Wrapf(readErr, "read policy %s", path)
			}

			relPath := relativePolicyPath(root, path)
			if err := s.verifyChecksum(relPath, content); err != nil {
				return err
			}

			modules[path] = content
			continue
		}

		// Check for data files
		if s.opts.includeData && ext == ".json" {
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return coreerrs.Wrapf(readErr, "read data file %s", path)
			}

			var jsonData any
			if unmarshalErr := json.Unmarshal(content, &jsonData); unmarshalErr != nil {
				return coreerrs.Wrapf(unmarshalErr, "parse JSON data %s", path)
			}

			if *data == nil {
				*data = make(map[string]any)
			}

			// Use relative path as the data key
			relPath, relErr := filepath.Rel(root, path)
			if relErr != nil {
				// Fall back to base name if rel path fails
				relPath = filepath.Base(path)
			}
			key := strings.TrimSuffix(relPath, ".json")
			key = strings.ReplaceAll(key, string(filepath.Separator), "/")
			(*data)[key] = jsonData
		}
	}

	return nil
}

// relativePolicyPath computes the relative path key used in the checksums map.
func relativePolicyPath(root, filePath string) string {
	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		return filepath.Base(filePath)
	}
	return filepath.ToSlash(rel)
}

// verifyChecksum checks a single file against the checksums manifest.
// It is a no-op when checksums are not configured (s.opts.checksums == nil).
func (s *Source) verifyChecksum(relPath string, content []byte) error {
	if s.opts.checksums == nil {
		return nil
	}

	expected, ok := s.opts.checksums[relPath]
	if !ok {
		return coreerrs.Wrapf(ErrUnexpectedPolicyFile, "%s", relPath)
	}

	actual := corehash.SHA256HexBytes(content)

	if actual != expected {
		return coreerrs.Wrapf(ErrChecksumMismatch, "%s (expected %s, got %s)", relPath, expected, actual)
	}

	return nil
}

// verifyAllChecksumsCovered ensures every entry in the checksums manifest has a corresponding loaded module.
// It is a no-op when checksums are not configured.
func (s *Source) verifyAllChecksumsCovered(loaded map[string]string) error {
	if s.opts.checksums == nil {
		return nil
	}

	for path := range s.opts.checksums {
		if _, ok := loaded[path]; !ok {
			return coreerrs.Wrapf(ErrMissingPolicyFile, "%s", path)
		}
	}

	return nil
}

// Close releases resources held by the source.
func (s *Source) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	s.logger.Debug("filesystem source closed", slog.String("path", s.path))

	return nil
}

// Path returns the configured filesystem path.
func (s *Source) Path() string {
	return s.path
}

// Extensions returns the file extensions being watched as a slice.
func (s *Source) Extensions() []string {
	return slices.Collect(maps.Keys(s.extensions))
}

// ExtensionsIter returns an iterator over the file extensions being watched.
// This is more efficient when you don't need a slice.
func (s *Source) ExtensionsIter() iter.Seq[string] {
	return maps.Keys(s.extensions)
}

// Compile-time interface check.
var _ opa.PolicySource = (*Source)(nil)
