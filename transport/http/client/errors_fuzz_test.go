// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func FuzzUnexpectedStatusError_Error(f *testing.F) {
	f.Add(404, "GET", "example.com", "/users")
	f.Add(500, "POST", "", "")
	f.Add(0, "", "", "")

	f.Fuzz(func(t *testing.T, status int, method, host, uri string) {
		err := UnexpectedStatusError{Status: status, Method: method, Host: host, URI: uri}
		assert.NotEqual(t, "", err.Error())
	})
}

func FuzzResponseSizeError_Error(f *testing.F) {
	f.Add(int64(100), int64(200))
	f.Add(int64(0), int64(0))

	f.Fuzz(func(t *testing.T, limit, size int64) {
		err := &ResponseSizeError{Limit: limit, Size: size}
		assert.NotEqual(t, "", err.Error())
	})
}
