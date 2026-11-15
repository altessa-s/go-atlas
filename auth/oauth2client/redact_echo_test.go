// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"net/http"
	"testing"
)

func TestErrorResponseOpaqueCredentialsWithheld(t *testing.T) {
	t.Parallel()
	runRedactionResponses(t, []redactionResponse{
		{
			name:        "bare client secret",
			status:      http.StatusBadRequest,
			contentType: "application/json",
			body:        `{"error":"invalid_client","error_description":"Rejected s3cr3t","error_uri":"s3cr3t"}`,
			wantInError: `error "invalid_client" (description: 15 bytes withheld) (uri: 6 bytes withheld)`,
		},
		{
			name:        "opaque refresh token",
			status:      http.StatusBadRequest,
			contentType: "application/json",
			body:        `{"error":"invalid_grant","error_description":"Rejected rt-123","error_uri":"rt-123"}`,
			wantInError: `error "invalid_grant" (description: 15 bytes withheld) (uri: 6 bytes withheld)`,
		},
	})
}
