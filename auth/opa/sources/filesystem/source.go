// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filesystem

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas-core/io/files"
	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/auth/opa/internal/checksum"
	"github.com/altessa-s/go-atlas/auth/opa/internal/jsondoc"

	coremaps "github.com/altessa-s/go-atlas-core/collections/maps"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
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
	checksums  *checksum.Manifest
	logger     *slog.Logger
	extensions *coremaps.ImmutableMap[string, struct{}]

	mu     sync.Mutex
	closed bool
}

// New creates a new filesystem policy source.
// The path can be a directory containing .rego files or a single .rego file.
// Configure a release manifest with [WithReleaseManifest] or [WithChecksums].
func New(path string, opts ...Option) (*Source, error) {
	o := newOptions(opts...)
	if o.releaseManifest == "" && o.checksums == nil {
		return nil, opa.ErrReleaseManifestRequired
	}
	if o.releaseManifest != "" && (!checksum.ValidPath(o.releaseManifest) || o.checksums != nil) {
		return nil, errors.New("invalid or conflicting filesystem release manifest configuration")
	}

	isDir := files.DirExists(path)
	isFile := files.FileExists(path)

	if !isDir && !isFile {
		return nil, fmt.Errorf("invalid path %q: path does not exist", path)
	}

	if !isDir && !strings.HasSuffix(path, ".rego") {
		return nil, fmt.Errorf("path must be a directory or .rego file: %s", path)
	}

	manifest := checksum.New(o.checksums)
	o.checksums = nil

	extensions := make(map[string]struct{}, len(o.extensions))
	for _, ext := range o.extensions {
		extensions[ext] = struct{}{}
	}

	return &Source{
		path:       path,
		opts:       o,
		checksums:  manifest,
		logger:     cmp.Or(o.logger, slog.New(slog.DiscardHandler)),
		extensions: coremaps.NewImmutableMap(extensions),
	}, nil
}

// Name returns the source identifier.
func (s *Source) Name() string {
	return "filesystem:" + s.path
}

// Fetch reads all policy files from the configured path.
func (s *Source) Fetch(_ context.Context) (*opa.PolicyBundle, error) {
	return s.fetch(nil)
}

func (s *Source) fetch(readFile func(string) ([]byte, error)) (*opa.PolicyBundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, opa.ErrSourceClosed
	}

	modules := make(map[string][]byte)
	var data map[string]any
	loadedRelPaths := make(map[string]string)

	info, err := os.Stat(s.path)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "stat path")
	}

	base := s.path
	if !info.IsDir() {
		base = filepath.Dir(s.path)
	}
	read := readFile
	if read == nil {
		read = os.ReadFile
	}
	manifestPath := filepath.Join(base, filepath.FromSlash(s.opts.releaseManifest))
	root, manifest, err := s.readManifest(manifestPath, read)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		if err := s.walkDirectory(base, root, manifestPath, readFile, manifest, modules, &data, loadedRelPaths); err != nil {
			return nil, err
		}
	} else {
		if root != "" {
			return nil, coreerrs.WrapOperation(coreerrs.ErrInvalidArgument, "release root requires a policy directory")
		}
		content, err := read(s.path)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "read policy file")
		}

		relPath := relativePolicyPath(filepath.Dir(s.path), s.path)
		if err := manifest.Verify(relPath, content, ErrChecksumMismatch, ErrUnexpectedPolicyFile); err != nil {
			return nil, err
		}

		loadedRelPaths[relPath] = s.path
		modules[s.path] = content
	}

	if err := manifest.Covered(loadedRelPaths, ErrMissingPolicyFile); err != nil {
		return nil, err
	}

	if len(modules) == 0 {
		return nil, coreerrs.Wrapf(opa.ErrNoPolicyFiles, "%s", s.path)
	}

	bundle, revisionErr := opa.NewPolicyBundleWithData(modules, data)
	if revisionErr != nil {
		return nil, revisionErr
	}

	s.logger.Debug("fetched policy bundle",
		slog.String("path", s.path),
		slog.Int("modules", len(modules)),
		slog.String("revision", bundle.Revision))

	return bundle, nil
}

func (s *Source) walkDirectory(base, root, manifestPath string, readFile func(string) ([]byte, error),
	manifest *checksum.Manifest, modules map[string][]byte, data *map[string]any, loaded map[string]string) error {
	if root == "" {
		if readFile == nil {
			readFile = os.ReadFile
		}
		entries := files.Walk(base, files.WithRecursive(), files.WithFileTypes(files.FileTypeRegular))
		return s.loadFiles(entries, base, manifestPath, readFile, manifest, modules, data, loaded)
	}

	baseDir, err := os.OpenRoot(base)
	if err != nil {
		return coreerrs.WrapOperation(err, "open policy base")
	}
	defer func() { _ = baseDir.Close() }()
	releaseDir, err := baseDir.OpenRoot(filepath.FromSlash(root))
	if err != nil {
		return coreerrs.WrapOperation(err, "open filesystem release root")
	}
	defer func() { _ = releaseDir.Close() }()

	dir := filepath.Join(base, filepath.FromSlash(root))
	if readFile == nil {
		readFile = func(name string) ([]byte, error) {
			return releaseDir.ReadFile(filepath.FromSlash(relativePolicyPath(dir, name)))
		}
	}
	return s.loadFiles(releaseFiles(releaseDir.FS(), dir), dir, manifestPath, readFile, manifest, modules, data, loaded)
}

func releaseFiles(root fs.FS, dir string) iter.Seq2[files.Entry, error] {
	return func(yield func(files.Entry, error) bool) {
		stopped := errors.New("stop walking release directory")
		err := fs.WalkDir(root, ".", func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.Type().IsRegular() && !yield(files.Entry{
				Path: filepath.Join(dir, filepath.FromSlash(name)), DirEntry: entry,
			}, nil) {
				return stopped
			}
			return nil
		})
		if err != nil && !errors.Is(err, stopped) {
			yield(files.Entry{}, err)
		}
	}
}

func (s *Source) loadFiles(entries iter.Seq2[files.Entry, error], root, manifestPath string, readFile func(string) ([]byte, error),
	manifest *checksum.Manifest, modules map[string][]byte, data *map[string]any, loaded map[string]string) error {
	for entry, err := range entries {
		if err != nil {
			return err
		}

		path := entry.Path
		ext := filepath.Ext(entry.Name())
		isModule := s.extensions.Contains(ext)
		isData := s.opts.includeData && ext == ".json"
		if !isModule && !isData {
			continue
		}

		relPath := relativePolicyPath(root, path)
		if path == manifestPath {
			continue
		}
		content, readErr := readFile(path)
		if readErr != nil {
			return coreerrs.Wrapf(readErr, "read policy file %s", path)
		}
		if err := manifest.Verify(relPath, content, ErrChecksumMismatch, ErrUnexpectedPolicyFile); err != nil {
			return err
		}
		loaded[relPath] = path

		if isModule {
			modules[path] = content
			continue
		}
		jsonData, err := jsondoc.Decode(content)
		if err != nil {
			return coreerrs.Wrapf(err, "parse JSON data %s", path)
		}
		if *data == nil {
			*data = make(map[string]any)
		}
		(*data)[strings.TrimSuffix(relPath, ".json")] = jsonData
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

func (s *Source) readManifest(name string, readFile func(string) ([]byte, error)) (string, *checksum.Manifest, error) {
	if s.opts.releaseManifest == "" {
		return "", s.checksums, nil
	}
	content, err := readFile(name)
	if err != nil {
		return "", nil, coreerrs.WrapOperation(err, "read filesystem release manifest")
	}
	root, entries, err := checksum.DecodeRelease[string](content)
	if err != nil {
		return "", nil, coreerrs.WrapOperation(err, "decode filesystem release manifest")
	}
	for name, digest := range entries {
		if !checksum.ValidDigest(digest) {
			return "", nil, fmt.Errorf("invalid SHA-256 digest for %q", name)
		}
	}
	return root, checksum.New(entries), nil
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
	return slices.Collect(s.extensions.Keys())
}

// ExtensionsIter returns an iterator over the file extensions being watched.
// This is more efficient when you don't need a slice.
func (s *Source) ExtensionsIter() iter.Seq[string] {
	return s.extensions.Keys()
}

// Compile-time interface check.
var _ opa.PolicySource = (*Source)(nil)
