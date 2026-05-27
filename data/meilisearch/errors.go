// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"errors"

	msdk "github.com/meilisearch/meilisearch-go"
)

// Meilisearch API error codes classified explicitly. See:
// https://www.meilisearch.com/docs/reference/errors/error_codes
const (
	// errCodeIndexNotFound is returned when the requested index does not exist.
	errCodeIndexNotFound = "index_not_found"
	// errCodeIndexAlreadyExists is returned when creating an index that already exists.
	errCodeIndexAlreadyExists = "index_already_exists"
)

// Sentinel errors for the meilisearch package. Returned by the client
// when a Meilisearch API call surfaces one of the classified codes.
// Callers should branch on these with [errors.Is]:
//
//	if errors.Is(err, meilisearch.ErrIndexNotFound) {
//	    // create the index lazily
//	}
//
// The [IsErrorIndexNotFound] / [IsErrorIndexAlreadyExists] predicates
// remain available for callers that already use them; both forms agree.
var (
	// ErrIndexNotFound is wrapped into the returned error when Meilisearch
	// responds with the "index_not_found" code.
	ErrIndexNotFound = errors.New("meilisearch: index not found")

	// ErrIndexAlreadyExists is wrapped into the returned error when
	// Meilisearch responds with the "index_already_exists" code.
	ErrIndexAlreadyExists = errors.New("meilisearch: index already exists")
)

// IsErrorIndexNotFound reports whether err is a Meilisearch
// "index_not_found" response.
func IsErrorIndexNotFound(err error) bool {
	return errors.Is(err, ErrIndexNotFound) || hasMeilisearchAPICode(err, errCodeIndexNotFound)
}

// IsErrorIndexAlreadyExists reports whether err is a Meilisearch
// "index_already_exists" response.
func IsErrorIndexAlreadyExists(err error) bool {
	return errors.Is(err, ErrIndexAlreadyExists) || hasMeilisearchAPICode(err, errCodeIndexAlreadyExists)
}

// hasMeilisearchAPICode returns true when err unwraps to a [msdk.Error]
// whose MeilisearchApiError.Code matches code.
func hasMeilisearchAPICode(err error, code string) bool {
	var meiliErr *msdk.Error
	if !errors.As(err, &meiliErr) {
		return false
	}
	return meiliErr.MeilisearchApiError.Code == code
}

// classifySDKError returns the matching sentinel for known Meilisearch
// API error codes, joined with the original SDK error so both the
// sentinel (for [errors.Is]) and the original detail (for logs /
// trace) are reachable from the returned value. Returns nil when err
// is nil or does not match any classified code.
func classifySDKError(err error) error {
	var meiliErr *msdk.Error
	if err == nil || !errors.As(err, &meiliErr) {
		return nil
	}
	switch meiliErr.MeilisearchApiError.Code {
	case errCodeIndexNotFound:
		return errors.Join(ErrIndexNotFound, err)
	case errCodeIndexAlreadyExists:
		return errors.Join(ErrIndexAlreadyExists, err)
	default:
		return nil
	}
}
