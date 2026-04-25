// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package responder

import (
	"net/http"
)

// WriteError writes a structured error response using the ErrorWriter from context.
// If no ErrorWriter is available in the context, falls back to http.Error.
//
// This function is intended for middleware that want explicit control over
// error responses, allowing them to pass actual error types instead of strings.
//
// Example:
//
//	if r.ContentLength > maxSize {
//	    responder.WriteError(w, r, ErrBodyTooLarge, http.StatusRequestEntityTooLarge)
//	    return
//	}
func WriteError(w http.ResponseWriter, r *http.Request, err error, statusCode int) error {
	writer := FromContext(r.Context())
	if writer == nil {
		// Fallback to plain text if no writer in context
		http.Error(w, err.Error(), statusCode)
		return nil
	}
	return writer.WriteError(w, r, err, statusCode)
}
