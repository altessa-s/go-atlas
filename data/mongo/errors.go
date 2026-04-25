// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"errors"

	"go.mongodb.org/mongo-driver/v2/mongo"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// MongoDB Error Codes for Helpers
const (
	MongoErrorCodeNamespaceNotFound = 26 // NamespaceNotFound
	MongoErrorCodeIndexNotFound     = 27 // IndexNotFound
	// MongoErrorCodeCollectionExists indicates a namespace (collection/database) already exists
	MongoErrorCodeCollectionExists = 48
	// MongoErrorCodeDuplicateKey indicates a duplicate key error during insert/update operations
	MongoErrorCodeDuplicateKey = 11000
	// MongoErrorCodeRetryable indicates a transient error that can be retried (WriteConflict)
	MongoErrorCodeRetryable = 112

	// Transaction-specific error codes
	// MongoErrorCodeTransientTransaction indicates a transient transaction error (NoSuchTransaction)
	MongoErrorCodeTransientTransaction = 251
	// MongoErrorCodeTransactionTooLarge indicates the transaction exceeded size limits
	MongoErrorCodeTransactionTooLarge = 17
	// MongoErrorCodeWriteConflict indicates a write conflict during transaction (same as MongoErrorCodeRetryable)
	MongoErrorCodeWriteConflict = 112
	// MongoErrorCodeMaxTimeMSExpired indicates the operation exceeded the specified time limit
	MongoErrorCodeMaxTimeMSExpired = 50
	// MongoErrorCodeLockTimeout indicates a lock acquisition timeout
	MongoErrorCodeLockTimeout = 216
)

// DuplicateFields represents a set of field names that caused a duplicate key error.
// It provides methods to check if specific fields were involved in the duplication.
type DuplicateFields struct {
	fields map[string]struct{}
}

// Contains returns true if the specified field name is part of the duplicate key error.
// It returns false if the receiver is nil, fields map is nil, or the field is not found.
//
// Parameters:
//   - field: The field name to check for duplication
//
// Returns:
//   - bool: True if the field was involved in the duplicate key error
func (d *DuplicateFields) Contains(field string) bool {
	if d == nil || d.fields == nil {
		return false
	}

	_, ok := d.fields[field]
	return ok
}

// IsErrorDuplicate analyzes an error to determine if it's a MongoDB duplicate key error.
// It extracts the field names involved in the duplication from the error details.
//
// Parameters:
//   - err: The error to analyze
//
// Returns:
//   - bool: True if the error is a duplicate key error
//   - *DuplicateFields: The fields involved in the duplication, or nil if extraction fails
func IsErrorDuplicate(err error) (bool, *DuplicateFields) {
	if mongo.IsDuplicateKeyError(err) {
		if e, ok := coreerrs.AsType[mongo.WriteException](err); ok && len(e.WriteErrors) > 0 {
			els, err := e.WriteErrors[0].Raw.Lookup("keyPattern").Document().Elements()
			if err == nil {
				var fields = &DuplicateFields{}
				fields.fields = make(map[string]struct{}, len(els))
				for _, el := range els {
					fields.fields[el.Key()] = struct{}{}
				}
				return true, fields
			}
		}
		return true, nil
	}
	return false, nil
}

// IsErrorCollectionNotFound checks if the error is a collection not found error.
// Returns true if the error is a collection not found error.
func IsErrorCollectionNotFound(err error) bool {
	if se, ok := coreerrs.AsType[mongo.ServerError](err); ok {
		// NamespaceNotFound
		return se.HasErrorCode(MongoErrorCodeNamespaceNotFound)
	}
	return false
}

// IsErrorIndexNotFound checks if the error is an index not found error.
func IsErrorIndexNotFound(err error) bool {
	if se, ok := coreerrs.AsType[mongo.ServerError](err); ok {
		// IndexNotFound
		return se.HasErrorCode(MongoErrorCodeIndexNotFound)
	}
	return false
}

// IsTransientTransaction checks if the error is a transient transaction error that can be retried.
// This function provides enhanced error classification for transaction-specific error codes.
func IsTransientTransaction(err error) bool {
	if coreerrs.IsContextCanceledOrDeadlineExceeded(err) {
		return false
	}

	if mongo.IsNetworkError(err) || mongo.IsTimeout(err) {
		return true
	}

	if cmdErr, ok := coreerrs.AsType[mongo.CommandError](err); ok {
		return cmdErr.HasErrorCode(MongoErrorCodeWriteConflict) ||
			cmdErr.HasErrorCode(MongoErrorCodeTransientTransaction) ||
			cmdErr.HasErrorCode(MongoErrorCodeLockTimeout) ||
			cmdErr.HasErrorCode(MongoErrorCodeMaxTimeMSExpired)
	}

	if writeErr, ok := coreerrs.AsType[mongo.WriteException](err); ok && len(writeErr.WriteErrors) > 0 {
		for _, we := range writeErr.WriteErrors {
			if we.Code == MongoErrorCodeWriteConflict {
				return true
			}
		}
	}

	return false
}

// Package errors
var (
	// ErrEncryptionNotEnabled is returned when encryption operations are attempted without encryption configured
	ErrEncryptionNotEnabled = errors.New("encryption not enabled")
	// ErrEncryptionUnknownAlg is returned when an unsupported encryption algorithm is specified
	ErrEncryptionUnknownAlg = errors.New("unknown encryption algorithm")
	// ErrUnsupportedVersion is returned when the MongoDB version is below the minimum required
	ErrUnsupportedVersion = errors.New("unsupported mongo version")
	// ErrDataKeyNotFound is returned when a requested data encryption key is not found
	ErrDataKeyNotFound = errors.New("data key not found")
	// ErrSortFieldNotAllowed is returned by [ParseSortStringStrict] when the
	// sort string references a field that is not in the caller-supplied
	// allowlist or starts with `$` (an operator key like `$natural`, which
	// would force a full collection scan and is never a legitimate target
	// for user-driven input).
	ErrSortFieldNotAllowed = errors.New("sort field not allowed")
)
