// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"net/http"
	"strconv"

	"github.com/altessa-s/go-atlas/core/text/strings"
)

// Pre-computed interned strings for common HTTP status codes.
// These are used to avoid repeated string allocations when logging.
var internedStatusCodes = func() map[int]string {
	codes := []int{
		http.StatusContinue,
		http.StatusSwitchingProtocols,
		http.StatusProcessing,
		http.StatusEarlyHints,
		http.StatusOK,
		http.StatusCreated,
		http.StatusAccepted,
		http.StatusNonAuthoritativeInfo,
		http.StatusNoContent,
		http.StatusResetContent,
		http.StatusPartialContent,
		http.StatusMultiStatus,
		http.StatusAlreadyReported,
		http.StatusIMUsed,
		http.StatusMultipleChoices,
		http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusNotModified,
		http.StatusUseProxy,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect,
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusPaymentRequired,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
		http.StatusNotAcceptable,
		http.StatusProxyAuthRequired,
		http.StatusRequestTimeout,
		http.StatusConflict,
		http.StatusGone,
		http.StatusLengthRequired,
		http.StatusPreconditionFailed,
		http.StatusRequestEntityTooLarge,
		http.StatusRequestURITooLong,
		http.StatusUnsupportedMediaType,
		http.StatusRequestedRangeNotSatisfiable,
		http.StatusExpectationFailed,
		http.StatusTeapot,
		http.StatusMisdirectedRequest,
		http.StatusUnprocessableEntity,
		http.StatusLocked,
		http.StatusFailedDependency,
		http.StatusTooEarly,
		http.StatusUpgradeRequired,
		http.StatusPreconditionRequired,
		http.StatusTooManyRequests,
		http.StatusRequestHeaderFieldsTooLarge,
		http.StatusUnavailableForLegalReasons,
		http.StatusInternalServerError,
		http.StatusNotImplemented,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		http.StatusHTTPVersionNotSupported,
		http.StatusVariantAlsoNegotiates,
		http.StatusInsufficientStorage,
		http.StatusLoopDetected,
		http.StatusNotExtended,
		http.StatusNetworkAuthenticationRequired,
	}

	m := make(map[int]string, len(codes))
	for _, code := range codes {
		m[code] = strings.InternString(strconv.Itoa(code))
	}
	return m
}()

// DefaultLogStatusCodes contains common HTTP status codes that should be logged
// by default. Used by [New] when no custom [WithLogResponseCodes] option is
// provided. Pre-allocated at initialization to avoid per-request allocation.
var DefaultLogStatusCodes = []int{
	http.StatusOK,
	http.StatusCreated,
	http.StatusAccepted,
	http.StatusNoContent,
	http.StatusMovedPermanently,
	http.StatusFound,
	http.StatusNotModified,
	http.StatusBadRequest,
	http.StatusUnauthorized,
	http.StatusForbidden,
	http.StatusNotFound,
	http.StatusMethodNotAllowed,
	http.StatusConflict,
	http.StatusGone,
	http.StatusUnprocessableEntity,
	http.StatusTooManyRequests,
	http.StatusInternalServerError,
	http.StatusNotImplemented,
	http.StatusBadGateway,
	http.StatusServiceUnavailable,
	http.StatusGatewayTimeout,
}

// DefaultLogStatusCodesSet is a pre-computed set for O(1) lookup of
// [DefaultLogStatusCodes]. Used by the middleware's shouldLogStatusCode
// hot path.
var DefaultLogStatusCodesSet = func() map[int]struct{} {
	m := make(map[int]struct{}, len(DefaultLogStatusCodes))
	for _, code := range DefaultLogStatusCodes {
		m[code] = struct{}{}
	}
	return m
}()

// InternedStatusString returns the interned string representation of an HTTP
// status code. Common codes are pre-computed at init time; unknown codes
// are interned on first use via [strings.InternString].
func InternedStatusString(code int) string {
	if s, ok := internedStatusCodes[code]; ok {
		return s
	}
	return strings.InternString(strconv.Itoa(code))
}
