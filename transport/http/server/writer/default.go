// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import (
	"errors"
	"net/http"
)

// Default is the default [Builder] implementation.
// It wraps responses in a [Response] envelope with Data or Error fields.
// Create instances with [NewDefault].
//
// Default is safe for concurrent use. It is immutable after construction
// and [Default.Build] does not modify any internal state.
//
// When building error responses, Default checks for the following interfaces
// on the error value (via [errors.As] to support wrapped errors):
//   - [Coder]: extracts machine-readable error code via Code()
//   - [Messager]: extracts user-facing message via Message()
//   - [HTTPStatuser]: extracts HTTP status code via HTTPStatus()
//
// If an [ErrorConverter] is configured via [WithDefaultBuilderErrorConverter],
// it takes priority over interface extraction.
type Default struct {
	options *defaultBuilderOptions
}

// NewDefault creates a new Default builder with the specified options.
// By default, returns HTTP 500 for all errors.
//
// Example:
//
//	// Using interface-based error extraction
//	b := builder.NewDefault()
//
//	// Using custom error converter (takes priority over interfaces)
//	b := builder.NewDefault(builder.WithDefaultBuilderErrorConverter(converter))
func NewDefault(opts ...DefaultBuilderOption) *Default {
	return &Default{
		options: newDefaultBuilderOptions(opts...),
	}
}

// Build constructs a Response from data.
// Errors are converted using interfaces or error converter;
// other data is wrapped in Response.Data.
func (b *Default) Build(_ *http.Request, data any) (any, int) {
	if err, ok := data.(error); ok {
		return b.buildError(err)
	}
	return &Response{Data: data}, http.StatusOK
}

// buildError converts an error to a Response with Error field.
// Priority: ErrorConverter > Interface extraction
func (b *Default) buildError(err error) (any, int) {
	// ErrorConverter takes priority (explicit configuration)
	if b.options.errorConverter != nil {
		e, status := b.options.errorConverter(err)
		if status == 0 {
			status = http.StatusInternalServerError
		}
		return &Response{Error: &e}, status
	}

	// Extract from interfaces using errors.As (supports wrapped errors).
	// Status is resolved before message so the message fallback can mirror
	// the chosen status (e.g. 503 -> "Service Unavailable") instead of
	// leaking err.Error() — which is often a database driver string, a
	// file path, or another internal detail the client must not see.
	code := b.extractCode(err)
	status := b.extractStatus(err)
	message := b.extractMessage(err, status)

	return &Response{Error: &Error{Code: code, Message: message}}, status
}

// extractCode attempts to extract error code from Coder interface.
// Uses errors.As to support wrapped errors.
func (b *Default) extractCode(err error) string {
	var coder Coder
	if errors.As(err, &coder) {
		if code := coder.Code(); code != "" {
			return code
		}
	}
	return ""
}

// extractMessage returns the user-facing message for err. It uses the
// [Messager] interface when implemented (the explicit opt-in for callers
// that want to expose a message), otherwise it returns the generic HTTP
// status text for status. err.Error() is intentionally never surfaced
// here: arbitrary errors routinely contain database driver output, file
// paths, validation diagnostics with sensitive context, etc., and leaking
// them across the trust boundary is what this fallback exists to prevent.
// Callers that need the original error in observability should log it
// before invoking the writer.
func (b *Default) extractMessage(err error, status int) string {
	var messager Messager
	if errors.As(err, &messager) {
		if msg := messager.Message(); msg != "" {
			return msg
		}
	}
	if msg := http.StatusText(status); msg != "" {
		return msg
	}
	return http.StatusText(http.StatusInternalServerError)
}

// extractStatus attempts to extract HTTP status from HTTPStatuser interface.
// Falls back to 500 Internal Server Error if HTTPStatuser is not implemented.
func (b *Default) extractStatus(err error) int {
	var s HTTPStatuser //nolint:misspell // statuser is Go convention for interface implementer
	if errors.As(err, &s) {
		if status := s.HTTPStatus(); status > 0 {
			return status
		}
	}
	return http.StatusInternalServerError
}
