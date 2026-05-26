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

// IsErrorIndexNotFound reports whether err is a Meilisearch
// "index_not_found" response.
func IsErrorIndexNotFound(err error) bool {
	return hasMeilisearchAPICode(err, errCodeIndexNotFound)
}

// IsErrorIndexAlreadyExists reports whether err is a Meilisearch
// "index_already_exists" response.
func IsErrorIndexAlreadyExists(err error) bool {
	return hasMeilisearchAPICode(err, errCodeIndexAlreadyExists)
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
