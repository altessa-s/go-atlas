// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"errors"

	"github.com/altessa-s/go-aip/pagination/pagetoken"

	"github.com/altessa-s/go-atlas-core/collections/slices"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	pagingconfig "github.com/altessa-s/go-atlas/config/paging"
)

// errConfigRequired reports a nil paging configuration.
var errConfigRequired = errors.New("pagetoken/factory: paging configuration is required")

// CodecBuilder assembles a [pagetoken.Codec] from a [pagingconfig.Config].
type CodecBuilder struct {
	cfg  *pagingconfig.Config
	opts []pagetoken.Option
}

// New creates a [CodecBuilder] for cfg. A nil cfg is accepted; the error
// surfaces at [CodecBuilder.Build] time.
func New(cfg *pagingconfig.Config) *CodecBuilder {
	return &CodecBuilder{cfg: cfg}
}

// UseOptions appends codec options applied after the configured ones (for
// example pagetoken.WithClock in tests).
func (b *CodecBuilder) UseOptions(opts ...pagetoken.Option) *CodecBuilder {
	b.opts = append(b.opts, opts...)
	return b
}

// Build decodes the configured keys under their encoding and returns the
// codec: tokens are signed with signingKey, verified with it and every
// previous key, and expire after tokenTTL (0 keeps [pagetoken.DefaultTTL]).
// encryption.enabled issues encrypted tokens ([pagetoken.WithEncryption]);
// encryption.strict also rejects signed ones ([pagetoken.WithStrictEncryption]).
func (b *CodecBuilder) Build() (*pagetoken.Codec, error) {
	if b.cfg == nil {
		return nil, errConfigRequired
	}
	current, previous, err := b.cfg.Keys()
	if err != nil {
		return nil, coreerrs.Wrap(err, "decode page-token keys")
	}
	opts := []pagetoken.Option{pagetoken.WithPreviousKeys(previous...)}
	opts = slices.AppendIf(opts, b.cfg.TokenTTL > 0, pagetoken.WithTTL(b.cfg.TokenTTL))
	opts = slices.AppendIf(opts, b.cfg.Encryption.Enabled, pagetoken.WithEncryption(true))
	opts = slices.AppendIf(opts, b.cfg.Encryption.Strict, pagetoken.WithStrictEncryption(true))
	codec, err := pagetoken.New(current, append(opts, b.opts...)...)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create page-token codec")
	}
	return codec, nil
}
