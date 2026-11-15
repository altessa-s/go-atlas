// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storedoc

import (
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Classify wraps a BSON document decoding failure with category, preserving
// both errors for errors.Is and errors.As. It recognizes *bson.DecodeError
// and bson.ValueDecoderError anywhere in the error chain.
//
// The caller owns category (for example, its corrupt-record sentinel).
// Nil errors, a nil category, errors already matching category, and unrelated
// failures (query, connectivity, cursor iteration or missing documents) are
// returned unchanged. This helper does not inspect error messages.
func Classify(err, category error) error {
	if err == nil || category == nil || errors.Is(err, category) {
		return err
	}
	if _, ok := errors.AsType[*bson.DecodeError](err); ok {
		return fmt.Errorf("%w: %w", category, err)
	}
	if _, ok := errors.AsType[bson.ValueDecoderError](err); ok {
		return fmt.Errorf("%w: %w", category, err)
	}
	return err
}
