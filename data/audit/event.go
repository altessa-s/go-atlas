// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import "time"

// EventType represents the category of an audit event.
type EventType string

const (
	// EventTypeAPIRequest records an inbound API call (gRPC or HTTP).
	EventTypeAPIRequest EventType = "api.request"

	// EventTypeBusinessEvent records a domain-level business action.
	EventTypeBusinessEvent EventType = "business.event"

	// EventTypeDataChange records a create, update, or delete on a data resource.
	EventTypeDataChange EventType = "data.change"

	// EventTypeAuth records authentication or authorization decisions.
	EventTypeAuth EventType = "auth"

	// EventTypeSystem records internal system operations (e.g. migrations, scheduled jobs).
	EventTypeSystem EventType = "system"
)

// Action represents the operation performed.
type Action string

const (
	// ActionCreate records the creation of a new resource.
	ActionCreate Action = "create"
	// ActionRead records a read or retrieval of a resource.
	ActionRead Action = "read"
	// ActionUpdate records a modification to an existing resource.
	ActionUpdate Action = "update"
	// ActionDelete records the removal of a resource.
	ActionDelete Action = "delete"
	// ActionExecute records the execution of an operation or command.
	ActionExecute Action = "execute"
	// ActionLogin records a user authentication event.
	ActionLogin Action = "login"
	// ActionLogout records a user session termination.
	ActionLogout Action = "logout"
	// ActionGrant records a permission or role grant.
	ActionGrant Action = "grant"
	// ActionRevoke records a permission or role revocation.
	ActionRevoke Action = "revoke"
)

// ActorType represents the type of entity performing the action.
type ActorType string

const (
	// ActorTypeUser represents a human end-user.
	ActorTypeUser ActorType = "user"

	// ActorTypeService represents another service or microservice.
	ActorTypeService ActorType = "service"

	// ActorTypeSystem represents the platform itself (scheduled jobs, migrations).
	ActorTypeSystem ActorType = "system"

	// ActorTypeAPIKey represents an external caller authenticated by API key.
	ActorTypeAPIKey ActorType = "api_key"

	// ActorTypeAnonymous represents an unauthenticated caller.
	ActorTypeAnonymous ActorType = "anonymous"
)

// ResultStatus represents the outcome of the action.
type ResultStatus string

const (
	// ResultStatusSuccess indicates the action completed successfully.
	ResultStatusSuccess ResultStatus = "success"

	// ResultStatusFailure indicates a client-side failure (e.g. validation error).
	ResultStatusFailure ResultStatus = "failure"

	// ResultStatusDenied indicates an authorization or authentication rejection.
	ResultStatusDenied ResultStatus = "denied"

	// ResultStatusError indicates an unexpected server-side error.
	ResultStatusError ResultStatus = "error"
)

// SortOrder defines the sort direction for queries.
type SortOrder string

const (
	// SortOrderAsc sorts results in ascending (oldest first) order.
	SortOrderAsc SortOrder = "asc"
	// SortOrderDesc sorts results in descending (newest first) order.
	SortOrderDesc SortOrder = "desc"
)

// Event represents a single audit event.
type Event struct {
	ID        string         `json:"id" bson:"_id"`
	Type      EventType      `json:"type" bson:"type"`
	Action    Action         `json:"action" bson:"action"`
	Actor     Actor          `json:"actor" bson:"actor"`
	Resource  Resource       `json:"resource" bson:"resource"`
	Result    Result         `json:"result" bson:"result"`
	Context   EventContext   `json:"context" bson:"context"`
	Service   ServiceInfo    `json:"service" bson:"service"`
	Timestamp time.Time      `json:"timestamp" bson:"timestamp"`
	Duration  time.Duration  `json:"duration,omitempty" bson:"duration,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty" bson:"metadata,omitempty"`
}

// Actor represents the entity performing the audited action.
type Actor struct {
	Type      ActorType      `json:"type" bson:"type"`
	ID        string         `json:"id" bson:"id"`
	Name      string         `json:"name,omitempty" bson:"name,omitempty"`
	Email     string         `json:"email,omitempty" bson:"email,omitempty"`
	IP        string         `json:"ip,omitempty" bson:"ip,omitempty"`
	UserAgent string         `json:"user_agent,omitempty" bson:"user_agent,omitempty"`
	Roles     []string       `json:"roles,omitempty" bson:"roles,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty" bson:"metadata,omitempty"`
}

// Resource represents the target of the audited action.
type Resource struct {
	Type       string           `json:"type" bson:"type"`
	ID         string           `json:"id" bson:"id"`
	Name       string           `json:"name,omitempty" bson:"name,omitempty"`
	Path       string           `json:"path,omitempty" bson:"path,omitempty"`
	Changes    *ResourceChanges `json:"changes,omitempty" bson:"changes,omitempty"`
	Attributes map[string]any   `json:"attributes,omitempty" bson:"attributes,omitempty"`
}

// ResourceChanges describes what changed on a resource.
type ResourceChanges struct {
	Before map[string]any `json:"before,omitempty" bson:"before,omitempty"`
	After  map[string]any `json:"after,omitempty" bson:"after,omitempty"`
	Fields []string       `json:"fields,omitempty" bson:"fields,omitempty"`
}

// Result describes the outcome of the audited action.
type Result struct {
	Status  ResultStatus `json:"status" bson:"status"`
	Code    int          `json:"code,omitempty" bson:"code,omitempty"`
	Message string       `json:"message,omitempty" bson:"message,omitempty"`
	Error   *ResultError `json:"error,omitempty" bson:"error,omitempty"`
}

// ResultError holds error details for failed actions.
type ResultError struct {
	Code    string `json:"code,omitempty" bson:"code,omitempty"`
	Message string `json:"message,omitempty" bson:"message,omitempty"`
}

// EventContext holds request correlation identifiers.
type EventContext struct {
	RequestID     string `json:"request_id,omitempty" bson:"request_id,omitempty"`
	TraceID       string `json:"trace_id,omitempty" bson:"trace_id,omitempty"`
	SpanID        string `json:"span_id,omitempty" bson:"span_id,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty" bson:"correlation_id,omitempty"`
}

// ServiceInfo identifies the service that generated the event.
type ServiceInfo struct {
	Name     string `json:"name" bson:"name"`
	Version  string `json:"version,omitempty" bson:"version,omitempty"`
	Instance string `json:"instance,omitempty" bson:"instance,omitempty"`
}
