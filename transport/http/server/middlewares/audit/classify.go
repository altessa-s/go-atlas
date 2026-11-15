// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"net/http"

	"github.com/altessa-s/go-atlas/audit"
)

// classifyHTTPStatus maps an HTTP status code to an audit Result.
// 200-399 = success, 400-499 = failure, 500+ = error.
func classifyHTTPStatus(statusCode int) audit.Result {
	r := audit.Result{Status: audit.ResultStatusSuccess, Code: statusCode}
	if statusCode >= http.StatusBadRequest {
		r.Status = audit.ResultStatusFailure
	}
	if statusCode >= http.StatusInternalServerError {
		r.Status = audit.ResultStatusError
	}
	return r
}

// httpMethodToAction maps an HTTP method string to an audit Action.
func httpMethodToAction(method string) audit.Action {
	switch method {
	case http.MethodPost:
		return audit.ActionCreate
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return audit.ActionRead
	case http.MethodPut, http.MethodPatch:
		return audit.ActionUpdate
	case http.MethodDelete:
		return audit.ActionDelete
	default:
		return audit.ActionExecute
	}
}
