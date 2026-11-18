// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"net/http"
	"strconv"

	"github.com/altessa-s/go-atlas-core/text/strings/intern"

	coremaps "github.com/altessa-s/go-atlas-core/collections/maps"
)

// Keep the default response status on a direct path for successful requests.
var internedStatusOK = intern.String(strconv.Itoa(http.StatusOK))

// Pre-computed interned strings for other common HTTP status codes.
// These are used to avoid repeated string allocations when logging.
var internedStatusCodes = func() *coremaps.ImmutableMap[int, string] {
	codes := []int{
		http.StatusContinue,
		http.StatusSwitchingProtocols,
		http.StatusProcessing,
		http.StatusEarlyHints,
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
		m[code] = intern.String(strconv.Itoa(code))
	}
	return coremaps.NewImmutableMap(m)
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
	http.StatusSeeOther,
	http.StatusNotModified,
	http.StatusTemporaryRedirect,
	http.StatusPermanentRedirect,
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
var DefaultLogStatusCodesSet = newStatusCodeSet(DefaultLogStatusCodes)

func newStatusCodeSet(codes []int) *coremaps.ImmutableMap[int, struct{}] {
	m := make(map[int]struct{}, len(codes))
	for _, code := range codes {
		m[code] = struct{}{}
	}
	return coremaps.NewImmutableMap(m)
}

// InternedStatusString returns the interned string representation of an HTTP
// status code. Common codes are pre-computed at init time; unknown codes
// are interned on first use via [intern.String].
func InternedStatusString(code int) string {
	if code == http.StatusOK {
		return internedStatusOK
	}
	if s, ok := internedStatusCodes.Get(code); ok {
		return s
	}
	return intern.String(strconv.Itoa(code))
}
