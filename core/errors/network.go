// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errors

import (
	"crypto/x509"
	"errors"
	"net"
	"net/url"
	"regexp"
	"syscall"
)

// IsNetworkError reports whether err or any error in its chain satisfies the
// [net.Error] interface. It uses [AsType] to traverse the full error chain,
// including multi-errors from [errors.Join]. Returns false when err is nil.
//
// For more specific checks see [IsRequestTimeoutError], [IsConnectionRefused],
// and [IsURLError].
//
// Example:
//
//	if errors.IsNetworkError(err) { /* handle network issue */ }
func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := AsType[net.Error](err) //nolint:errcheck // only checking ok
	return ok
}

// IsRequestTimeoutError reports whether err represents a network or URL request
// timeout. It checks for [net.Error] with Timeout() == true, and for [*url.Error]
// with Timeout() == true. The error chain is traversed via [AsType].
// Returns false when err is nil.
//
// See also [IsNetworkError] for a broader network error check.
//
// Example:
//
//	if errors.IsRequestTimeoutError(err) { /* retry or fail */ }
func IsRequestTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	if netErr, ok := AsType[net.Error](err); ok {
		return netErr.Timeout()
	}

	if urlErr, ok := AsType[*url.Error](err); ok {
		return urlErr.Timeout()
	}

	return false
}

var (
	// redirectsErrorRe matches the error string for exceeding the maximum HTTP redirects.
	redirectsErrorRe = regexp.MustCompile(`stopped after \d+ redirects\z`)

	// schemeErrorRe matches the error string for an unsupported URL protocol scheme.
	schemeErrorRe = regexp.MustCompile(`unsupported protocol scheme`)
)

// IsURLError reports whether err or any error in its chain is a [*url.Error].
// This covers HTTP client errors such as DNS resolution failures, TLS handshake
// errors, and redirect problems. The error chain is traversed via [AsType].
// Returns false when err is nil.
//
// For more specific URL error conditions see [IsResourceRedirects],
// [IsUnsupportedProtocolScheme], and [IsCertUnknownAuthority].
//
// Example:
//
//	if errors.IsURLError(err) { /* check URL format */ }
func IsURLError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := AsType[*url.Error](err) //nolint:errcheck // only checking ok
	return ok
}

// IsResourceRedirects reports whether err is a [*url.Error] whose message matches
// the standard library's "stopped after N redirects" pattern. This typically occurs
// when an HTTP client follows too many consecutive redirects.
// Returns false when err is nil or when the error is not a [*url.Error].
//
// Example:
//
//	if errors.IsResourceRedirects(err) { /* too many redirects */ }
func IsResourceRedirects(err error) bool {
	if err == nil {
		return false
	}
	if urlErr, ok := AsType[*url.Error](err); ok && urlErr.Err != nil {
		return redirectsErrorRe.MatchString(urlErr.Err.Error())
	}
	return false
}

// IsUnsupportedProtocolScheme reports whether err is a [*url.Error] whose message
// indicates an unsupported protocol scheme (e.g., "ftp" when only "http" and "https"
// are accepted). Returns false when err is nil or when the error is not a [*url.Error].
//
// Example:
//
//	if errors.IsUnsupportedProtocolScheme(err) { /* use http or https */ }
func IsUnsupportedProtocolScheme(err error) bool {
	if err == nil {
		return false
	}
	if urlErr, ok := AsType[*url.Error](err); ok && urlErr.Err != nil {
		return schemeErrorRe.MatchString(urlErr.Err.Error())
	}
	return false
}

// IsCertUnknownAuthority reports whether err is a [*url.Error] wrapping an
// [x509.UnknownAuthorityError]. This indicates that the server presented a TLS
// certificate signed by an authority not in the client's trust store.
// Returns false when err is nil or when the underlying error is not an
// [x509.UnknownAuthorityError].
//
// Example:
//
//	if errors.IsCertUnknownAuthority(err) { /* add CA to trust store */ }
func IsCertUnknownAuthority(err error) bool {
	if err == nil {
		return false
	}
	if urlErr, ok := AsType[*url.Error](err); ok {
		_, found := AsType[x509.UnknownAuthorityError](urlErr.Err) //nolint:errcheck // only checking found
		return found
	}
	return false
}

// IsConnectionRefused reports whether err indicates that the remote host actively
// refused the connection ([syscall.ECONNREFUSED]). It recursively unwraps
// [*url.Error] and [*net.OpError] layers that the standard library's HTTP client
// and net.Dial produce, checking for ECONNREFUSED at each level.
// Returns false when err is nil.
//
// Example:
//
//	if errors.IsConnectionRefused(err) { /* service unavailable */ }
func IsConnectionRefused(err error) bool {
	if err == nil {
		return false
	}

	if urlErr, ok := AsType[*url.Error](err); ok {
		return IsConnectionRefused(urlErr.Unwrap())
	}

	if netErr, ok := AsType[*net.OpError](err); ok {
		if netErr.Op == "dial" || netErr.Op == "read" {
			if syscallErr, ok := AsType[syscall.Errno](netErr.Err); ok {
				return errors.Is(syscallErr, syscall.ECONNREFUSED)
			}
			return false
		}
		return IsConnectionRefused(netErr.Unwrap())
	}

	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}

	return false
}
