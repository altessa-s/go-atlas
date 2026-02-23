// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errors_test

import (
	"errors"
	"net/url"
	"testing"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

func FuzzRegexMatchers(f *testing.F) {
	f.Add("stopped after 10 redirects")
	f.Add("unsupported protocol scheme")
	f.Add("random error string")

	f.Fuzz(func(t *testing.T, s string) {
		err := &url.Error{Op: "Get", URL: "http://test", Err: errors.New(s)}

		// Just ensure these don't panic on random input
		_ = coreerrs.IsResourceRedirects(err)
		_ = coreerrs.IsUnsupportedProtocolScheme(err)
	})
}
