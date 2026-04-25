// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package codec

// KeyDecoder defines an interface for encoding and decoding secret keys.
// Implementations of this interface allow transforming keys into a format suitable
// for use in secret storage systems and restoring them to their original form.
type KeyDecoder interface {
	// Decode transforms an encoded key back to its original format.
	// Returns the decoded key and an error if decoding fails.
	Decode(key string) (string, error)

	// Encode transforms a key into a format suitable for storage.
	// Returns the encoded key and an error if encoding fails.
	Encode(key string) (string, error)
}

// ValueDecoder defines an interface for encoding and decoding secret values.
// The type parameter T defines the type of the value, which must
// conform to the secrets.ValueConstraint restriction.
type ValueDecoder[T any] interface {
	// Decode transforms binary data into a value of type T.
	// Parameters:
	//   - data: binary data that needs to be decoded
	// Returns the decoded value of type T and an error if decoding fails.
	Decode(data []byte) (T, error)

	// Encode transforms a value of type T into binary data for storage.
	// Parameters:
	//   - value: the value of type T that needs to be encoded
	// Returns the encoded binary data and an error if encoding fails.
	Encode(value T) ([]byte, error)
}
