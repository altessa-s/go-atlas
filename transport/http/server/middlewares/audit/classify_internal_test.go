// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit"
)

func TestClassifyHTTPStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		code   int
		status audit.ResultStatus
	}{
		{"ok_200", http.StatusOK, audit.ResultStatusSuccess},
		{"created_201", http.StatusCreated, audit.ResultStatusSuccess},
		{"redirect_301", http.StatusMovedPermanently, audit.ResultStatusSuccess},
		{"bad_request_400", http.StatusBadRequest, audit.ResultStatusFailure},
		{"forbidden_403", http.StatusForbidden, audit.ResultStatusFailure},
		{"not_found_404", http.StatusNotFound, audit.ResultStatusFailure},
		{"internal_error_500", http.StatusInternalServerError, audit.ResultStatusError},
		{"service_unavailable_503", http.StatusServiceUnavailable, audit.ResultStatusError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := classifyHTTPStatus(tt.code)
			require.Equal(t, tt.status, result.Status)
			require.Equal(t, tt.code, result.Code)
		})
	}
}

func TestHTTPMethodToAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method string
		action audit.Action
	}{
		{http.MethodPost, audit.ActionCreate},
		{http.MethodGet, audit.ActionRead},
		{http.MethodHead, audit.ActionRead},
		{http.MethodOptions, audit.ActionRead},
		{http.MethodPut, audit.ActionUpdate},
		{http.MethodPatch, audit.ActionUpdate},
		{http.MethodDelete, audit.ActionDelete},
		{"CUSTOM", audit.ActionExecute},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.action, httpMethodToAction(tt.method))
		})
	}
}
