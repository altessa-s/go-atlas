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

func TestIsURLError(t *testing.T) {
	urlErr := &url.Error{Op: "Get", URL: "http://x", Err: errors.New("fail")}
	if !coreerrs.IsURLError(urlErr) {
		t.Error("should detect url.Error")
	}
	if coreerrs.IsURLError(errors.New("plain")) {
		t.Error("should not detect plain error")
	}
	if coreerrs.IsURLError(nil) {
		t.Error("should not detect nil")
	}
}

func TestIsCertUnknownAuthority(t *testing.T) {
	// Non-url.Error should not match
	if coreerrs.IsCertUnknownAuthority(errors.New("other")) {
		t.Error("should not detect plain error")
	}
	if coreerrs.IsCertUnknownAuthority(nil) {
		t.Error("should not detect nil")
	}
	// url.Error with non-cert inner error
	urlErr := &url.Error{Op: "Get", URL: "https://x", Err: errors.New("other")}
	if coreerrs.IsCertUnknownAuthority(urlErr) {
		t.Error("should not detect url.Error with plain inner error")
	}
}

func TestIsNetworkError_Nil(t *testing.T) {
	if coreerrs.IsNetworkError(nil) {
		t.Error("should not detect nil")
	}
}

func TestIsRequestTimeoutError_NonTimeout(t *testing.T) {
	if coreerrs.IsRequestTimeoutError(nil) {
		t.Error("should not detect nil")
	}
	if coreerrs.IsRequestTimeoutError(errors.New("plain")) {
		t.Error("should not detect plain error")
	}
}

func TestIsResourceRedirects_Nil(t *testing.T) {
	if coreerrs.IsResourceRedirects(nil) {
		t.Error("should not detect nil")
	}
	if coreerrs.IsResourceRedirects(errors.New("no redirects")) {
		t.Error("should not detect non-redirect error")
	}
}

func TestIsUnsupportedProtocolScheme_Nil(t *testing.T) {
	if coreerrs.IsUnsupportedProtocolScheme(nil) {
		t.Error("should not detect nil")
	}
}

func TestIsConnectionRefused_Nil(t *testing.T) {
	if coreerrs.IsConnectionRefused(nil) {
		t.Error("should not detect nil")
	}
	if coreerrs.IsConnectionRefused(errors.New("other")) {
		t.Error("should not detect plain error")
	}
}

func TestWrapNilCases(t *testing.T) {
	if coreerrs.Wrap(nil, "ctx") != nil {
		t.Error("Wrap(nil) should be nil")
	}
	if coreerrs.WrapOperation(nil, "op") != nil {
		t.Error("WrapOperation(nil) should be nil")
	}
	if coreerrs.WrapField(nil, "f") != nil {
		t.Error("WrapField(nil) should be nil")
	}
	if coreerrs.WrapOperationWithContext(nil, "op", "ctx") != nil {
		t.Error("WrapOperationWithContext(nil) should be nil")
	}
}
