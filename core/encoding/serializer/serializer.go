// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package serializer

// Serializer defines a symmetric encoding contract: values passed to
// [Serializer.Serialize] must be recoverable by [Serializer.Deserialize]
// with the same implementation. This interface is used by cache, uniq,
// and other packages that need format-agnostic value marshaling.
//
// Implementations must be stateless and safe for concurrent use from
// multiple goroutines.
//
// The canonical implementation is [JSON].
type Serializer interface {
	// Serialize encodes data into a byte representation suitable for
	// storage or transmission. The exact wire format depends on the
	// implementation (JSON, Protocol Buffers, etc.). It returns an
	// error if data contains values that the underlying codec cannot
	// represent.
	Serialize(data any) ([]byte, error)

	// Deserialize decodes bytes produced by [Serializer.Serialize] into
	// out, which must be a non-nil pointer to the target type.
	// It returns an error if d is malformed or incompatible with the
	// type of out.
	Deserialize(d []byte, out any) error
}
