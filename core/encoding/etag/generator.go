// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package etag

import (
	"io"
	"unsafe"

	corehash "github.com/altessa-s/go-atlas/core/encoding/hash"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Generator produces strong content tags using a configurable hash (default
// SHA-256, see [DefaultNewHash]). Build it with [NewGenerator]; the zero value
// is not usable.
//
// A Generator is safe for concurrent use: every call allocates a fresh
// [hash.Hash] via the configured [NewHashFunc], so no state is shared.
type Generator struct {
	opts *options
}

// NewGenerator returns a Generator configured by opts. With no options it uses
// [DefaultNewHash] (SHA-256).
func NewGenerator(opts ...Option) *Generator {
	return &Generator{opts: newOptions(opts...)}
}

// Hash returns a strong tag whose value is the lowercase hex digest of data.
func (g *Generator) Hash(data []byte) Tag {
	h := g.opts.newHash()
	h.Write(data)
	return Strong(corehash.HexSum(h))
}

// HashString returns a strong tag whose value is the lowercase hex digest of
// s. It avoids copying s with a zero-copy string-to-[]byte conversion, valid
// because the hash only reads the bytes for the duration of the call (see
// core/encoding/hash for the full safety rationale).
func (g *Generator) HashString(s string) Tag {
	h := g.opts.newHash()
	if s != "" {
		// #nosec G103 -- zero-copy read-only hashing, bytes never escape.
		h.Write(unsafe.Slice(unsafe.StringData(s), len(s)))
	}
	return Strong(corehash.HexSum(h))
}

// HashReader returns a strong tag for the full contents of r, streaming it
// through the hash without buffering the whole body. It returns an error
// wrapping any read failure from r.
func (g *Generator) HashReader(r io.Reader) (Tag, error) {
	h := g.opts.newHash()
	if _, err := io.Copy(h, r); err != nil {
		return Tag{}, coreerrs.Wrap(err, "etag: hash reader")
	}
	return Strong(corehash.HexSum(h)), nil
}
