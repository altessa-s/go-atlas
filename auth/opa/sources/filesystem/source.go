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

	"github.com/fsnotify/fsnotify"

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
// It reads .rego files from a directory and optionally watches for changes.
type Source struct {
	path       string
	opts       *options
	logger     *slog.Logger
	extensions map[string]struct{}

	mu       sync.Mutex
	watcher  *fsnotify.Watcher
	watching bool
	watchCh  chan struct{}
	stopCh   chan struct{}
	closed   bool
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
		return nil, fmt.Errorf("%w: %s", opa.ErrNoPolicyFiles, s.path)
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
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		ext := filepath.Ext(info.Name())

		// Check for policy files
		if _, ok := s.extensions[ext]; ok {
			content, err := os.ReadFile(path)
			if err != nil {
				return coreerrs.Wrapf(err, "read policy %s", path)
			}

			relPath := relativePolicyPath(root, path)
			if err := s.verifyChecksum(relPath, content); err != nil {
				return err
			}

			modules[path] = content
			return nil
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

		return nil
	})
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
		return fmt.Errorf("%w: %s", ErrUnexpectedPolicyFile, relPath)
	}

	actual := corehash.SHA256HexBytes(content)

	if actual != expected {
		return fmt.Errorf("%w: %s (expected %s, got %s)", ErrChecksumMismatch, relPath, expected, actual)
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
			return fmt.Errorf("%w: %s", ErrMissingPolicyFile, path)
		}
	}

	return nil
}

// Watch starts watching for file changes and returns a channel that signals updates.
func (s *Source) Watch(ctx context.Context) (<-chan struct{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, opa.ErrSourceClosed
	}

	if s.watching {
		return s.watchCh, nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create watcher")
	}

	// Add the path and all subdirectories
	info, err := os.Stat(s.path)
	if err != nil {
		_ = watcher.Close()
		return nil, coreerrs.WrapOperation(err, "stat path")
	}

	if info.IsDir() {
		if err := s.addWatchRecursive(watcher, s.path); err != nil {
			_ = watcher.Close()
			return nil, err
		}
	} else {
		if err := watcher.Add(filepath.Dir(s.path)); err != nil {
			_ = watcher.Close()
			return nil, coreerrs.WrapOperation(err, "watch directory")
		}
	}

	s.watcher = watcher
	s.watchCh = make(chan struct{}, 1)
	s.stopCh = make(chan struct{})
	s.watching = true

	go s.watchLoop(ctx)

	s.logger.Info("started watching for policy changes", slog.String("path", s.path))

	return s.watchCh, nil
}

// addWatchRecursive adds the directory and all subdirectories to the watcher.
func (s *Source) addWatchRecursive(watcher *fsnotify.Watcher, path string) error {
	return filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := watcher.Add(p); err != nil {
				return coreerrs.Wrapf(err, "watch %s", p)
			}
		}
		return nil
	})
}

// watchLoop handles fsnotify events and signals changes.
func (s *Source) watchLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case event, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			if s.isRelevantEvent(event) {
				s.logger.Debug("detected file change",
					slog.String("path", event.Name),
					slog.String("op", event.Op.String()))
				s.signal()

				// Handle new directories
				if event.Op&fsnotify.Create != 0 {
					if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
						if err := s.watcher.Add(event.Name); err != nil {
							s.logger.Warn("failed to watch new directory",
								slog.String("path", event.Name),
								slog.Any("error", err))
						}
					}
				}
			}
		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			s.logger.Error("watcher error", slog.Any("error", err))
		}
	}
}

// isRelevantEvent checks if the event is for a file we care about.
func (s *Source) isRelevantEvent(event fsnotify.Event) bool {
	if event.Op&fsnotify.Chmod != 0 {
		return false // Ignore permission changes
	}

	ext := filepath.Ext(event.Name)
	if _, ok := s.extensions[ext]; ok {
		return true
	}

	if s.opts.includeData && ext == ".json" {
		return true
	}

	// Also trigger on directory changes (new directories may contain policies)
	if event.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			return true
		}
	}

	return false
}

// signal sends a notification that policies may have changed.
func (s *Source) signal() {
	select {
	case s.watchCh <- struct{}{}:
	default:
		// Channel already has a pending signal
	}
}

// Close releases resources and stops watching.
func (s *Source) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	if s.watching {
		close(s.stopCh)
		if err := s.watcher.Close(); err != nil {
			return coreerrs.WrapOperation(err, "close watcher")
		}
		close(s.watchCh)
		s.watching = false
	}

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
