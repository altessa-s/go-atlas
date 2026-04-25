// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errors_test

import (
	"errors" // Standard errors
	"net"
	"net/url"
	"syscall"
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

func TestNetworkChecks(t *testing.T) {
	t.Run("IsNetworkError", func(t *testing.T) {
		netErr := &testhelpers.MockNetError{Msg: "net fail"}
		if !coreerrs.IsNetworkError(netErr) {
			t.Error("Should match net.Error")
		}
		if coreerrs.IsNetworkError(errors.New("plain")) {
			t.Error("Should not match plain error")
		}
	})

	t.Run("IsRequestTimeoutError", func(t *testing.T) {
		timeoutErr := &testhelpers.MockNetError{IsTimeout: true}
		if !coreerrs.IsRequestTimeoutError(timeoutErr) {
			t.Error("Should match timeout net.Error")
		}

		urlErr := &url.Error{Err: timeoutErr}
		if !coreerrs.IsRequestTimeoutError(urlErr) {
			t.Error("Should match wrapped timeout in url.Error")
		}
	})

	t.Run("IsConnectionRefused", func(t *testing.T) {
		refused := syscall.ECONNREFUSED
		if !coreerrs.IsConnectionRefused(refused) {
			t.Error("Should match ECONNREFUSED")
		}

		opErr := &net.OpError{Op: "dial", Err: refused}
		if !coreerrs.IsConnectionRefused(opErr) {
			t.Error("Should match ECONNREFUSED wrapped in OpError")
		}

		urlErr := &url.Error{Err: opErr}
		if !coreerrs.IsConnectionRefused(urlErr) {
			t.Error("Should match ECONNREFUSED wrapped in url.Error -> OpError")
		}
	})

	t.Run("RegexMatches", func(t *testing.T) {
		// IsUnsupportedProtocolScheme
		badProto := &url.Error{Err: errors.New("unsupported protocol scheme \"ftp\"")}
		if !coreerrs.IsUnsupportedProtocolScheme(badProto) {
			// Note: Regex in source matches "unsupported protocol scheme"
			// But standard library puts it in Err usually?
			// Actually url.Error.Error() stringifies it.
			// coreerrs checks MatchString(urlErr.Error())
			t.Error("Should match unsupported protocol scheme")
		}

		// IsResourceRedirects
		redirects := &url.Error{Err: errors.New("stopped after 10 redirects")}
		// Regex: `stopped after \d+ redirects\z`
		if !coreerrs.IsResourceRedirects(redirects) {
			t.Error("Should match redirects error")
		}
	})
}
