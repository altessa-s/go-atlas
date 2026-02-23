// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package codec

import (
	"errors"
	"iter"
	"slices"
	"strings"
	"sync"

	"github.com/aohorodnyk/mimeheader"

	"github.com/altessa-s/go-atlas/core/runtime/panics"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

var (
	// ErrNoCodecFound is returned by [Registry.Negotiate] when no registered
	// encoder matches the client's Accept header. Callers should fall back to
	// a default content type or return HTTP 406 Not Acceptable.
	ErrNoCodecFound = errors.New("no suitable codec found")

	// ErrCodecAlreadyRegistered is returned by [Registry.RegisterEncoder],
	// [Registry.RegisterDecoder], and [Registry.RegisterCodec] when the
	// given MIME type already has a registered encoder or decoder.
	ErrCodecAlreadyRegistered = errors.New("codec already registered for this MIME type")
)

// Registry maps MIME types to [Encoder] and [Decoder] implementations.
// All methods are safe for concurrent use; the registry is protected by
// a read-write mutex.
type Registry struct {
	encoders       map[string]Encoder
	decoders       map[string]Decoder
	availableTypes []string
	mu             sync.RWMutex
}

// NewRegistry creates an empty [Registry]. Use [DefaultRegistry] to obtain the
// global registry that includes the built-in JSON and XML codecs.
func NewRegistry() *Registry {
	return &Registry{
		encoders:       make(map[string]Encoder),
		decoders:       make(map[string]Decoder),
		availableTypes: make([]string, 0),
	}
}

// DefaultRegistry returns the global [Registry], initialized on first call.
// The init function populates it with JSON and XML codecs; additional codecs
// can be registered at any time.
var DefaultRegistry = sync.OnceValue(NewRegistry)

// RegisterEncoder registers an encoder for a MIME type.
func (r *Registry) RegisterEncoder(mimeType string, enc Encoder) error {
	if enc == nil {
		return errors.New("encoder cannot be nil")
	}

	mimeType = normalizeMimeType(mimeType)

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.encoders[mimeType]; exists {
		return ErrCodecAlreadyRegistered
	}

	r.encoders[mimeType] = enc
	r.availableTypes = append(r.availableTypes, mimeType)
	return nil
}

// RegisterDecoder registers a decoder for a MIME type.
func (r *Registry) RegisterDecoder(mimeType string, dec Decoder) error {
	if dec == nil {
		return errors.New("decoder cannot be nil")
	}

	mimeType = normalizeMimeType(mimeType)

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.decoders[mimeType]; exists {
		return ErrCodecAlreadyRegistered
	}

	r.decoders[mimeType] = dec
	return nil
}

// RegisterCodec registers both encoder and decoder for a codec's content type.
// If the decoder registration fails, the encoder registration is rolled back.
func (r *Registry) RegisterCodec(codec Codec) error {
	if codec == nil {
		return errors.New("codec cannot be nil")
	}

	mimeType := normalizeMimeType(codec.ContentType())

	if err := r.RegisterEncoder(mimeType, codec); err != nil {
		return err
	}

	if err := r.RegisterDecoder(mimeType, codec); err != nil {
		// Rollback encoder registration
		r.mu.Lock()
		delete(r.encoders, mimeType)
		// Remove from availableTypes
		r.availableTypes = coreslices.Delete(r.availableTypes, mimeType)
		r.mu.Unlock()
		return err
	}

	return nil
}

// MustRegisterCodec registers a codec and panics on failure.
func (r *Registry) MustRegisterCodec(codec Codec) {
	panics.MustError(r.RegisterCodec(codec))
}

// GetEncoder returns encoder for MIME type.
func (r *Registry) GetEncoder(mimeType string) (Encoder, bool) {
	mimeType = normalizeMimeType(mimeType)

	r.mu.RLock()
	defer r.mu.RUnlock()

	enc, ok := r.encoders[mimeType]
	return enc, ok
}

// GetDecoder returns decoder for MIME type.
func (r *Registry) GetDecoder(mimeType string) (Decoder, bool) {
	mimeType = normalizeMimeType(mimeType)

	r.mu.RLock()
	defer r.mu.RUnlock()

	dec, ok := r.decoders[mimeType]
	return dec, ok
}

// Negotiate performs content negotiation based on the HTTP Accept header value.
// It returns the best matching [Encoder], its MIME type, and nil on success.
// Returns [ErrNoCodecFound] when the Accept header is empty or no registered
// encoder matches the client's preferences.
func (r *Registry) Negotiate(acceptHeader string) (Encoder, string, error) {
	if acceptHeader == "" {
		return nil, "", ErrNoCodecFound
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Parse Accept header
	ah := mimeheader.ParseAcceptHeader(acceptHeader)

	// Negotiate best match using cached available types
	_, mimeType, matched := ah.Negotiate(r.availableTypes, "")
	if !matched {
		return nil, "", ErrNoCodecFound
	}

	encoder := r.encoders[mimeType]
	return encoder, mimeType, nil
}

// ListEncoders returns an iterator over registered encoder MIME types.
func (r *Registry) ListEncoders() iter.Seq[string] {
	return func(yield func(string) bool) {
		r.mu.RLock()
		// Copy slice to avoid holding lock during iteration
		types := slices.Clone(r.availableTypes)
		r.mu.RUnlock()

		for _, mimeType := range types {
			if !yield(mimeType) {
				return
			}
		}
	}
}

// ListDecoders returns an iterator over registered decoder MIME types.
func (r *Registry) ListDecoders() iter.Seq[string] {
	return func(yield func(string) bool) {
		r.mu.RLock()
		types := make([]string, 0, len(r.decoders))
		for mimeType := range r.decoders {
			types = append(types, mimeType)
		}
		r.mu.RUnlock()

		for _, mimeType := range types {
			if !yield(mimeType) {
				return
			}
		}
	}
}

// Encoders returns an iterator over registered encoders.
func (r *Registry) Encoders() iter.Seq2[string, Encoder] {
	return func(yield func(string, Encoder) bool) {
		r.mu.RLock()
		// Snapshot encoders to avoid holding lock during iteration
		// Use availableTypes for deterministic order
		snapshot := make([]struct {
			k string
			v Encoder
		}, len(r.availableTypes))

		for i, k := range r.availableTypes {
			snapshot[i] = struct {
				k string
				v Encoder
			}{k, r.encoders[k]}
		}
		r.mu.RUnlock()

		for _, e := range snapshot {
			if !yield(e.k, e.v) {
				return
			}
		}
	}
}

// Decoders returns an iterator over registered decoders.
func (r *Registry) Decoders() iter.Seq2[string, Decoder] {
	return func(yield func(string, Decoder) bool) {
		r.mu.RLock()
		// Snapshot decoders to avoid holding lock during iteration
		snapshot := make([]struct {
			k string
			v Decoder
		}, 0, len(r.decoders))

		for k, v := range r.decoders {
			snapshot = append(snapshot, struct {
				k string
				v Decoder
			}{k, v})
		}
		r.mu.RUnlock()

		for _, d := range snapshot {
			if !yield(d.k, d.v) {
				return
			}
		}
	}
}

// normalizeMimeType normalizes MIME type.
func normalizeMimeType(mimeType string) string {
	// Remove charset and parameters
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = mimeType[:idx]
	}

	// Trim whitespace and convert to lowercase using interned string pool
	// InternTrimString handles trimming, then InternLowerString converts to lowercase
	return corestrings.InternLowerString(corestrings.InternTrimString(mimeType))
}
