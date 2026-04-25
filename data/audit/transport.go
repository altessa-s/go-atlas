// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"net/http"
	"time"
)

// RequestInfo holds protocol-agnostic request metadata used to build audit events
// from transport-layer interceptors and middlewares.
type RequestInfo struct {
	Action       Action
	ResourceType string
	ResourcePath string
	Actor        Actor
	Context      EventContext
	StartTime    time.Time
	Duration     time.Duration
}

// BuildTransportEvent constructs an audit Event from a RequestInfo and a classify
// function that computes the Result (e.g. from HTTP status code or gRPC status).
func BuildTransportEvent(info RequestInfo, classify func() Result) *Event {
	return &Event{
		Type:      EventTypeAPIRequest,
		Action:    info.Action,
		Actor:     info.Actor,
		Resource:  Resource{Type: info.ResourceType, Path: info.ResourcePath},
		Result:    classify(),
		Context:   info.Context,
		Duration:  info.Duration,
		Timestamp: info.StartTime,
	}
}

// NewEventContext builds an EventContext from the given trace, span, and request IDs.
func NewEventContext(traceID, spanID, requestID string) EventContext {
	return EventContext{
		TraceID:   traceID,
		SpanID:    spanID,
		RequestID: requestID,
	}
}

// ClassifyHTTPStatus maps an HTTP status code to an audit Result.
// 200-399 = success, 400-499 = failure, 500+ = error.
func ClassifyHTTPStatus(statusCode int) Result {
	r := Result{Status: ResultStatusSuccess, Code: statusCode}
	if statusCode >= http.StatusBadRequest {
		r.Status = ResultStatusFailure
	}
	if statusCode >= http.StatusInternalServerError {
		r.Status = ResultStatusError
	}
	return r
}

// HTTPMethodToAction maps an HTTP method string to an audit Action.
func HTTPMethodToAction(method string) Action {
	switch method {
	case http.MethodPost:
		return ActionCreate
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return ActionRead
	case http.MethodPut, http.MethodPatch:
		return ActionUpdate
	case http.MethodDelete:
		return ActionDelete
	default:
		return ActionExecute
	}
}
