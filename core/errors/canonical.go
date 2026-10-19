// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errors

import "errors"

// Canonical domain sentinels. A domain or storage layer wraps one of them
// (fmt.Errorf("%w: ...", ErrNotFound)) to classify a failure independently of
// the transport, and callers test it with errors.Is. The gRPC errstatus
// interceptor maps them to status codes with fixed messages and stable
// ErrorInfo reasons (errstatus.WithCanonicalRules).
var (
	// ErrNotFound reports that a requested entity does not exist.
	ErrNotFound = errors.New("not found")

	// ErrAlreadyExists reports that an entity with the same identity
	// already exists.
	ErrAlreadyExists = errors.New("already exists")

	// ErrConflict reports an optimistic-concurrency conflict: the stored
	// entity's version (or etag) did not match the expected one, so the
	// write was not applied and the caller re-reads and retries.
	ErrConflict = errors.New("version conflict")

	// ErrInvalidArgument reports a semantically invalid request the schema
	// cannot express. Its wrapped text is meant for the client (the gRPC
	// mapping publishes it), so wrap it only with client-safe context; a
	// [FieldError] names the offending field instead.
	ErrInvalidArgument = errors.New("invalid argument")

	// ErrFailedPrecondition reports that the system is not in the state the
	// operation requires (for example, a resource that must be empty is
	// not); retrying does not help until the state changes.
	ErrFailedPrecondition = errors.New("failed precondition")

	// ErrUnavailable reports a transient failure of a dependency; the caller
	// retries later.
	ErrUnavailable = errors.New("unavailable")

	// ErrBusy reports that the same work is in progress elsewhere right now
	// (another worker holds its lease); the caller retries later instead of
	// duplicating it.
	ErrBusy = errors.New("busy")
)

// FieldError is an [ErrInvalidArgument] that names the offending field, so a
// transport reports a structured field violation instead of parsing the
// message. Classify it with errors.Is(err, ErrInvalidArgument) or
// errors.AsType[*FieldError](err).
//
// Err optionally keeps the failure that made the field invalid (a page-token
// or etag error, say) in the chain, so errors.Is still matches it, while
// Reason stays the text a client sees:
//
//	return &FieldError{Field: "pageToken", Reason: keyset.Classify(err).String(), Err: err}
type FieldError struct {
	// Field is the dot-separated path of the offending field. A domain
	// layer uses its own spelling (e.g. "attrs.law_code"); [MapFieldPath]
	// re-roots it under the API path of a request.
	Field string
	// Reason says what is wrong with the field's value.
	Reason string
	// Err is the underlying failure, if any. It is reachable with
	// errors.Is and errors.As but not part of the text.
	Err error
}

// NewFieldError returns a [FieldError] for field.
func NewFieldError(field, reason string) error {
	return &FieldError{Field: field, Reason: reason}
}

// Error renders "invalid argument: <field>: <reason>".
func (e *FieldError) Error() string {
	return ErrInvalidArgument.Error() + ": " + e.Field + ": " + e.Reason
}

// Unwrap returns [ErrInvalidArgument] and, when set, Err.
func (e *FieldError) Unwrap() []error {
	if e.Err == nil {
		return []error{ErrInvalidArgument}
	}
	return []error{ErrInvalidArgument, e.Err}
}

// MapFieldPath re-roots the [FieldError] err wraps under another path: the
// result matches errors.AsType[*FieldError] with Field mapPath(original
// Field) and the same Reason and Err, keeps matching every error err matches
// (errors.Is and errors.As reach the original chain) and keeps err's text.
// The original FieldError is never modified. Mapping a mapped error composes:
// the second mapper receives the first one's path.
//
// A handler uses it to name a domain field by the API path of its request
// (for example "base_url" under "provider" as "provider.baseUrl") before
// returning the error to a transport that reports field violations. A nil
// err, a nil mapPath or an err without a FieldError is returned unchanged.
func MapFieldPath(err error, mapPath func(field string) string) error {
	if err == nil || mapPath == nil {
		return err
	}
	fe, ok := errors.AsType[*FieldError](err)
	if !ok {
		return err
	}
	return &mappedFieldError{mapped: &FieldError{Field: mapPath(fe.Field), Reason: fe.Reason, Err: fe.Err}, err: err}
}

// mappedFieldError is the result of [MapFieldPath]: the re-rooted field
// error first, so errors.As finds it before the original, then the original
// chain.
type mappedFieldError struct {
	mapped *FieldError
	err    error
}

// Error returns the original error's text.
func (e *mappedFieldError) Error() string { return e.err.Error() }

// Unwrap returns the re-rooted field error and the original error.
func (e *mappedFieldError) Unwrap() []error { return []error{e.mapped, e.err} }
