// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	msdk "github.com/meilisearch/meilisearch-go"
)

// Meilisearch API error codes classified explicitly. See:
// https://www.meilisearch.com/docs/reference/errors/error_codes
const (
	// errCodeIndexNotFound is returned when the requested index does not exist.
	errCodeIndexNotFound = "index_not_found"
	// errCodeIndexAlreadyExists is returned when creating an index that already exists.
	errCodeIndexAlreadyExists = "index_already_exists"
)

// Sentinel errors for the meilisearch package. Index sentinels classify
// SDK errors internally. Use the predicates to recognize SDK error codes
// in returned errors:
//
//	if meilisearch.IsErrorIndexNotFound(err) {
//	    // create the index lazily
//	}
var (
	// ErrInvalidHost reports a malformed Meilisearch HTTP or HTTPS base URL.
	ErrInvalidHost = errors.New("meilisearch: invalid host URL")

	// ErrHealthCheck reports a failed or unavailable Meilisearch health probe.
	ErrHealthCheck = errors.New("meilisearch: health check failed")

	// ErrIndexNotFound classifies the SDK's "index_not_found" code
	// internally. Use [IsErrorIndexNotFound] to check returned errors.
	ErrIndexNotFound = errors.New("meilisearch: index not found")

	// ErrIndexAlreadyExists classifies the SDK's "index_already_exists" code
	// internally. Use [IsErrorIndexAlreadyExists] to check returned errors.
	ErrIndexAlreadyExists = errors.New("meilisearch: index already exists")

	// ErrTaskFailed is wrapped into the returned error when a Meilisearch
	// task observed by [Client.WaitForTask] reaches a terminal state other
	// than "succeeded". Match it with [errors.Is]; the diagnostic carries
	// the task UID, status, and validated error code/type without raw
	// server message text.
	ErrTaskFailed = errors.New("meilisearch: task failed")

	// ErrRequestFailed is the sanitized cause of an unclassified SDK failure.
	ErrRequestFailed = errors.New("meilisearch: request failed")

	// ErrInvalidDocumentID reports a missing, null, empty, or non-scalar
	// primary-key value encountered while enumerating document IDs.
	ErrInvalidDocumentID = errors.New("meilisearch: invalid or missing document primary key")
)

// IsErrorIndexNotFound reports whether err is a Meilisearch
// "index_not_found" response.
func IsErrorIndexNotFound(err error) bool {
	return errors.Is(err, ErrIndexNotFound) || hasMeilisearchAPICode(err, errCodeIndexNotFound)
}

// IsErrorIndexAlreadyExists reports whether err is a Meilisearch
// "index_already_exists" response.
func IsErrorIndexAlreadyExists(err error) bool {
	return errors.Is(err, ErrIndexAlreadyExists) || hasMeilisearchAPICode(err, errCodeIndexAlreadyExists)
}

type apiCodeCarrier interface {
	error
	meiliAPICode() string
}

// hasMeilisearchAPICode returns true when err carries a validated API code
// or unwraps to a [msdk.Error] whose APIError.Code matches code.
func hasMeilisearchAPICode(err error, code string) bool {
	if c, ok := errors.AsType[apiCodeCarrier](err); ok && c.meiliAPICode() == code {
		return true
	}
	meiliErr, ok := errors.AsType[*msdk.Error](err)
	if !ok {
		return false
	}
	return string(meiliErr.APIError.Code) == code
}

// classifySDKError returns the matching sentinel for known Meilisearch
// API error codes while preserving programmatic matching via [errors.Is].
// Returns nil when err is nil or does not match any classified code.
func classifySDKError(err error) error {
	if err == nil {
		return nil
	}
	var code string
	if c, ok := errors.AsType[apiCodeCarrier](err); ok {
		code = c.meiliAPICode()
	} else if meiliErr, ok := errors.AsType[*msdk.Error](err); ok {
		code = string(meiliErr.APIError.Code)
	}
	switch code {
	case errCodeIndexNotFound:
		return safeSDKErrorWithKind(ErrIndexNotFound, "index lookup", err)
	case errCodeIndexAlreadyExists:
		return safeSDKErrorWithKind(ErrIndexAlreadyExists, "index create", err)
	default:
		return nil
	}
}

type safeError struct {
	msg     string
	apiCode string
	kind    error
	cause   error
}

func (e *safeError) Error() string        { return e.msg }
func (e *safeError) GoString() string     { return e.msg }
func (e *safeError) meiliAPICode() string { return e.apiCode }

func (e *safeError) Unwrap() error        { return e.kind }
func (e *safeError) Is(target error) bool { return errors.Is(e.cause, target) }

func validIdentifier(s string) string {
	if s == "" || len(s) > 64 {
		return ""
	}
	for i := range len(s) {
		b := s[i]
		if (b < 'a' || b > 'z') && (b < '0' || b > '9') && b != '_' {
			return ""
		}
	}
	return s
}

// renderSafeDiagnostic formats Meilisearch task and HTTP errors from
// structured, validated fields only; APIError.Message and raw payloads
// are never included.
func renderSafeDiagnostic(op string, taskUID *int64, taskStatus string, httpStatus int, code, errType, errClass string) string {
	var b strings.Builder
	b.WriteString("meilisearch: ")
	b.WriteString(op)

	var details []string
	if taskUID != nil {
		details = append(details, fmt.Sprintf("task_uid=%d", *taskUID))
	}
	if taskStatus != "" {
		details = append(details, fmt.Sprintf("status=%q", cmp.Or(validIdentifier(taskStatus), "unknown")))
	}
	if httpStatus > 0 {
		details = append(details, fmt.Sprintf("http_status=%d", httpStatus))
	}
	if v := validIdentifier(code); v != "" {
		details = append(details, fmt.Sprintf("code=%q", v))
	}
	if v := validIdentifier(errType); v != "" {
		details = append(details, fmt.Sprintf("type=%q", v))
	}
	if errClass != "" {
		details = append(details, fmt.Sprintf("class=%q", errClass))
	}
	if len(details) > 0 {
		b.WriteString(" (")
		b.WriteString(strings.Join(details, ", "))
		b.WriteByte(')')
	}
	return b.String()
}

func safeSDKError(op string, err error) error {
	return safeSDKErrorWithKind(nil, op, err)
}

func safeSDKErrorWithKind(kind error, op string, err error) error {
	if err == nil {
		return nil
	}
	if existing, ok := errors.AsType[*safeError](err); ok && kind == nil {
		return existing
	}
	kind = cmp.Or(kind, ErrRequestFailed)
	if meiliErr, ok := errors.AsType[*msdk.Error](err); ok {
		code := validIdentifier(string(meiliErr.APIError.Code))
		errType := validIdentifier(meiliErr.APIError.Type)
		var class string
		if meiliErr.OriginError != nil {
			class = sdkErrorClass(meiliErr.OriginError)
		}
		return &safeError{
			msg:     renderSafeDiagnostic(op, nil, "", meiliErr.StatusCode, code, errType, class),
			apiCode: code,
			kind:    kind,
			cause:   err,
		}
	}
	return &safeError{
		msg:   renderSafeDiagnostic(op, nil, "", 0, "", "", sdkErrorClass(err)),
		kind:  kind,
		cause: err,
	}
}

func sdkErrorClass(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, io.ErrClosedPipe):
		return "closed_pipe"
	default:
		return "failed"
	}
}

func safeTaskError(op string, taskUID int64, task *msdk.Task) error {
	var status, code, errType string
	if task != nil {
		status = string(task.Status)
		code = validIdentifier(string(task.Error.Code))
		errType = validIdentifier(task.Error.Type)
	}
	return &safeError{
		msg:     renderSafeDiagnostic(op, &taskUID, status, 0, code, errType, ""),
		apiCode: code,
		kind:    ErrTaskFailed,
	}
}
