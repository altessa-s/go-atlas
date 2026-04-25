// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package embed

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/auth/opa"

	corehash "github.com/altessa-s/go-atlas/core/encoding/hash"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

var (
	// ErrChecksumMismatch is returned when a policy file's SHA-256 hash does not match the expected value.
	ErrChecksumMismatch = errors.New("policy file checksum mismatch")
	// ErrUnexpectedPolicyFile is returned when a policy file is found but has no entry in the checksums manifest.
	ErrUnexpectedPolicyFile = errors.New("policy file not in checksums manifest")
	// ErrMissingPolicyFile is returned when an entry in the checksums manifest has no corresponding file.
	ErrMissingPolicyFile = errors.New("expected policy file not found")
)

// Source implements opa.PolicySource for embed.FS-based policies.
// It reads .rego files from an fs.FS. Change detection is handled by the Manager.
type Source struct {
	fsys       fs.FS
	dir        string
	opts       *options
	logger     *slog.Logger
	extensions map[string]struct{}

	mu     sync.Mutex
	closed bool
}

// New creates a new embed policy source.
// The fsys is the filesystem to read from (typically embed.FS),
// dir is the directory within fsys containing policy files.
func New(fsys fs.FS, dir string, opts ...Option) (*Source, error) {
	if fsys == nil {
		return nil, fmt.Errorf("fs.FS is required")
	}

	if dir == "" {
		dir = "."
	}

	info, err := fs.Stat(fsys, dir)
	if err != nil {
		return nil, coreerrs.Wrapf(err, "invalid dir %q", dir)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("path %q is not a directory", dir)
	}

	o := newOptions(opts...)

	extensions := make(map[string]struct{}, len(o.extensions))
	for _, ext := range o.extensions {
		extensions[ext] = struct{}{}
	}

	return &Source{
		fsys:       fsys,
		dir:        dir,
		opts:       o,
		logger:     cmp.Or(o.logger, slog.New(slog.DiscardHandler)),
		extensions: extensions,
	}, nil
}

// Name returns the source identifier.
func (s *Source) Name() string {
	return "embed:" + s.dir
}

// Fetch reads all policy files from the configured fs.FS directory.
func (s *Source) Fetch(_ context.Context) (*opa.PolicyBundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, opa.ErrSourceClosed
	}

	modules := make(map[string][]byte)
	var data map[string]any

	if err := s.walkDirectory(s.dir, modules, &data); err != nil {
		return nil, err
	}

	loadedRelPaths := make(map[string]string, len(modules))
	for p := range modules {
		rel := relativePolicyPath(s.dir, p)
		loadedRelPaths[rel] = p
	}

	if err := s.verifyAllChecksumsCovered(loadedRelPaths); err != nil {
		return nil, err
	}

	if len(modules) == 0 {
		return nil, coreerrs.Wrapf(opa.ErrNoPolicyFiles, "%s", s.dir)
	}

	var bundle *opa.PolicyBundle
	if data != nil {
		bundle = opa.NewPolicyBundleWithData(modules, data)
	} else {
		bundle = opa.NewPolicyBundle(modules)
	}

	s.logger.Debug("fetched policy bundle",
		slog.String("dir", s.dir),
		slog.Int("modules", len(modules)),
		slog.String("revision", bundle.Revision))

	return bundle, nil
}

// walkDirectory recursively walks the directory and collects policy files.
func (s *Source) walkDirectory(root string, modules map[string][]byte, data *map[string]any) error {
	return fs.WalkDir(s.fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		ext := path.Ext(d.Name())

		if _, ok := s.extensions[ext]; ok {
			content, readErr := fs.ReadFile(s.fsys, p)
			if readErr != nil {
				return coreerrs.Wrapf(readErr, "read policy %s", p)
			}

			relPath := relativePolicyPath(root, p)
			if err := s.verifyChecksum(relPath, content); err != nil {
				return err
			}

			modules[p] = content
			return nil
		}

		if s.opts.includeData && ext == ".json" {
			content, readErr := fs.ReadFile(s.fsys, p)
			if readErr != nil {
				return coreerrs.Wrapf(readErr, "read data file %s", p)
			}

			var jsonData any
			if unmarshalErr := json.Unmarshal(content, &jsonData); unmarshalErr != nil {
				return coreerrs.Wrapf(unmarshalErr, "parse JSON data %s", p)
			}

			if *data == nil {
				*data = make(map[string]any)
			}

			relPath := relativePolicyPath(root, p)
			key := strings.TrimSuffix(relPath, ".json")
			(*data)[key] = jsonData
		}

		return nil
	})
}

// relativePolicyPath computes the relative path key from root to filePath.
// Uses forward slashes since fs.FS paths are always forward-slash separated.
func relativePolicyPath(root, filePath string) string {
	if root == "." || root == "" {
		return filePath
	}

	prefix := root + "/"
	if strings.HasPrefix(filePath, prefix) {
		return filePath[len(prefix):]
	}

	return filePath
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

	for p := range s.opts.checksums {
		if _, ok := loaded[p]; !ok {
			return coreerrs.Wrapf(ErrMissingPolicyFile, "%s", p)
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

	s.logger.Debug("embed source closed", slog.String("dir", s.dir))

	return nil
}

// Dir returns the configured directory.
func (s *Source) Dir() string {
	return s.dir
}

// Extensions returns the file extensions being loaded as a slice.
func (s *Source) Extensions() []string {
	return slices.Collect(maps.Keys(s.extensions))
}

// Compile-time interface check.
var _ opa.PolicySource = (*Source)(nil)
