// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package files

import (
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"strings"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// FileType represents categories of filesystem entries returned by [Walk].
// Multiple types can be combined as a bitmask.
type FileType uint8

const (
	// FileTypeRegular matches regular files (not directories, symlinks, devices).
	FileTypeRegular FileType = 1 << iota

	// FileTypeDir matches directories.
	FileTypeDir

	// FileTypeSymlink matches symbolic links.
	FileTypeSymlink

	// FileTypeOther matches anything that is not a regular file, directory or symlink
	// (named pipes, sockets, devices, irregular files).
	FileTypeOther
)

// FileTypeAny is a bitmask matching every supported file type.
const FileTypeAny = FileTypeRegular | FileTypeDir | FileTypeSymlink | FileTypeOther

// Entry represents a single filesystem entry yielded by [Walk].
type Entry struct {
	// Path is the full path of the entry, joined with the walk root.
	Path string

	// DirEntry is the underlying [fs.DirEntry] for the entry.
	DirEntry fs.DirEntry
}

// Name returns the base name of the entry.
func (e Entry) Name() string { return e.DirEntry.Name() }

// IsDir reports whether the entry is a directory.
func (e Entry) IsDir() bool { return e.DirEntry.IsDir() }

// Type returns the [FileType] classification of the entry.
func (e Entry) Type() FileType { return fileTypeFromMode(e.DirEntry.Type()) }

// Info returns the [fs.FileInfo] for the entry by calling DirEntry.Info.
func (e Entry) Info() (fs.FileInfo, error) { return e.DirEntry.Info() }

// Walk returns an iterator over filesystem entries under root.
//
// By default Walk lists only the top-level contents of root and yields every
// entry regardless of type. Use the With* options to filter by extension or
// file type, to enable recursion, to skip hidden entries, or to follow symlinks.
//
// # Error semantics
//
// Errors encountered while reading directories are yielded as a zero-value
// [Entry] with a non-nil error. Subdirectory read failures during recursion
// are yielded but do not abort the walk; the iteration continues with the
// next sibling. Consumers can stop iteration at any time by breaking out of
// the range loop.
//
// # Concurrency
//
// Walk itself is safe for concurrent invocation; each call captures its own
// options and produces an independent iterator. However, a single returned
// iterator must not be consumed by multiple goroutines simultaneously.
//
// # Recursion depth
//
// By default Walk visits only the immediate contents of root (depth 0).
// [WithRecursive] enables descent up to [DefaultMaxRecursionDepth] levels;
// [WithMaxDepth] sets an explicit bound; [WithUnboundedDepth] disables the
// limit entirely. The default cap exists primarily as a safety net against
// symlink cycles and adversarial trees.
//
// # Symlinks
//
// Without [WithFollowSymlinks], symbolic links are yielded as entries but
// Walk does not descend into directories they reference. With the option,
// symlinks resolving to directories are descended; symlink loops are not
// detected, so combine [WithFollowSymlinks] with a bounded depth (the
// default for [WithRecursive]) and use [WithUnboundedDepth] only on trees
// you fully trust.
//
// # Examples
//
// List every .so file in a directory (non-recursive):
//
//	for entry, err := range files.Walk("./plugins", files.WithExtensions(".so")) {
//	    if err != nil {
//	        return err
//	    }
//	    fmt.Println(entry.Path)
//	}
//
// Recursively find all regular YAML files, skipping hidden directories:
//
//	for entry, err := range files.Walk("/etc/app",
//	    files.WithRecursive(),
//	    files.WithExtensions(".yaml", ".yml"),
//	    files.WithFileTypes(files.FileTypeRegular),
//	    files.WithSkipHidden(),
//	) {
//	    // ...
//	}
func Walk(root string, opts ...WalkOption) iter.Seq2[Entry, error] {
	o := newWalkOptions(opts...)
	return func(yield func(Entry, error) bool) {
		walkDir(root, 0, o, yield)
	}
}

// walkDir reads dir and yields its entries. It returns false if the consumer
// requested early termination via yield, true otherwise.
func walkDir(dir string, depth int, o *walkOptions, yield func(Entry, error) bool) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return yield(Entry{}, coreerrs.WrapOperationWithContext(err, "read directory", dir))
	}

	for _, de := range entries {
		name := de.Name()

		if o.skipHidden && strings.HasPrefix(name, ".") {
			continue
		}

		path := filepath.Join(dir, name)
		entry := Entry{Path: path, DirEntry: de}

		if o.matches(entry) {
			if !yield(entry, nil) {
				return false
			}
		}

		// Determine whether to recurse into this entry.
		if o.maxDepth >= 0 && depth >= o.maxDepth {
			continue
		}

		isDir := de.IsDir()
		if !isDir && o.followSymlinks && de.Type()&fs.ModeSymlink != 0 {
			info, statErr := os.Stat(path)
			if statErr != nil {
				// The consumer opted into WithFollowSymlinks, so they
				// care about symlink targets. A failed stat means the
				// target is unreachable (broken link, EACCES on the
				// target, ELOOP for cyclic symlinks, etc.) and we
				// cannot tell whether it was a directory worth
				// descending into. Surface the failure as an error
				// entry rather than silently treating the symlink as
				// a non-directory — the symlink itself was already
				// yielded above (when matched), so the consumer can
				// correlate the error with the link they saw. After
				// surfacing the error continue with the next sibling;
				// a single broken link should not abort the walk.
				if !yield(Entry{Path: path, DirEntry: de},
					coreerrs.WrapOperationWithContext(statErr, "stat symlink target", path)) {
					return false
				}
				continue
			}
			if info.IsDir() {
				isDir = true
			}
		}
		if !isDir {
			continue
		}

		if !walkDir(path, depth+1, o, yield) {
			return false
		}
	}
	return true
}

// matches reports whether an entry passes the configured filters.
func (o *walkOptions) matches(e Entry) bool {
	if len(o.extensions) > 0 {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if _, ok := o.extensions[ext]; !ok {
			return false
		}
	}
	if o.types != 0 {
		if o.types&e.Type() == 0 {
			return false
		}
	}
	return true
}

// fileTypeFromMode classifies an [fs.FileMode] into a [FileType].
// Note: [fs.FileMode.IsRegular] returns true when no type bits are set
// (m&ModeType == 0), which covers both regular files and the zero value.
func fileTypeFromMode(m fs.FileMode) FileType {
	switch {
	case m.IsDir():
		return FileTypeDir
	case m&fs.ModeSymlink != 0:
		return FileTypeSymlink
	case m.IsRegular():
		return FileTypeRegular
	default:
		return FileTypeOther
	}
}
