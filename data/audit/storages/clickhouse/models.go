// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/altessa-s/go-atlas/data/audit"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// eventRow is the flat, column-per-field representation of an
// [audit.Event]. Free-form maps are carried as JSON strings so the table
// stays queryable on the fields that matter without a schema per tenant.
type eventRow struct {
	Timestamp time.Time
	ID        string
	Type      string
	Action    string

	ActorType      string
	ActorID        string
	ActorName      string
	ActorEmail     string
	ActorIP        string
	ActorUserAgent string
	ActorRoles     []string
	ActorMetadata  string

	ResourceType          string
	ResourceID            string
	ResourceName          string
	ResourcePath          string
	ResourceChangesBefore string
	ResourceChangesAfter  string
	ResourceChangesFields []string
	ResourceAttributes    string

	ResultStatus       string
	ResultCode         int64
	ResultMessage      string
	ResultErrorCode    string
	ResultErrorMessage string

	RequestID     string
	TraceID       string
	SpanID        string
	CorrelationID string

	ServiceName     string
	ServiceVersion  string
	ServiceInstance string

	DurationNS int64
	Metadata   string
}

// toRowInto converts an audit event into its row representation, writing
// into a caller-owned row so a batch can reuse one row for every event
// instead of allocating per event. dst is fully overwritten, including the
// fields that only some events populate.
//
// Three conversions are deliberately lossy and do not survive a round trip
// through [fromRow]:
//   - empty maps encode to the empty string and decode back to nil;
//   - empty slices decode back to nil;
//   - a non-nil but wholly empty Resource.Changes or Result.Error decodes
//     back to nil, since the row cannot distinguish "present and empty"
//     from "absent".
//
// Timestamp precision is a property of the column, not of this function:
// DateTime64(3) keeps milliseconds, so sub-millisecond detail is dropped on
// write and values read back are in UTC.
func toRowInto(dst *eventRow, e *audit.Event) error {
	actorMeta, err := encodeFields(e.Actor.Metadata)
	if err != nil {
		return coreerrs.WrapField(err, "actor.metadata")
	}

	attrs, err := encodeFields(e.Resource.Attributes)
	if err != nil {
		return coreerrs.WrapField(err, "resource.attributes")
	}

	meta, err := encodeFields(e.Metadata)
	if err != nil {
		return coreerrs.WrapField(err, "metadata")
	}

	// The literal assigns every field, which is what makes dst safe to reuse:
	// the ones only some events populate are reset to their zero value here
	// and filled in below.
	*dst = eventRow{
		Timestamp: e.Timestamp,
		ID:        e.ID,
		Type:      string(e.Type),
		Action:    string(e.Action),

		ActorType:      string(e.Actor.Type),
		ActorID:        e.Actor.ID,
		ActorName:      e.Actor.Name,
		ActorEmail:     e.Actor.Email,
		ActorIP:        e.Actor.IP,
		ActorUserAgent: e.Actor.UserAgent,
		ActorRoles:     e.Actor.Roles,
		ActorMetadata:  actorMeta,

		ResourceType:       e.Resource.Type,
		ResourceID:         e.Resource.ID,
		ResourceName:       e.Resource.Name,
		ResourcePath:       e.Resource.Path,
		ResourceAttributes: attrs,

		ResultStatus:  string(e.Result.Status),
		ResultCode:    int64(e.Result.Code),
		ResultMessage: e.Result.Message,

		RequestID:     e.Context.RequestID,
		TraceID:       e.Context.TraceID,
		SpanID:        e.Context.SpanID,
		CorrelationID: e.Context.CorrelationID,

		ServiceName:     e.Service.Name,
		ServiceVersion:  e.Service.Version,
		ServiceInstance: e.Service.Instance,

		DurationNS: int64(e.Duration),
		Metadata:   meta,
	}

	if c := e.Resource.Changes; c != nil {
		if dst.ResourceChangesBefore, err = encodeFields(c.Before); err != nil {
			return coreerrs.WrapField(err, "resource.changes.before")
		}
		if dst.ResourceChangesAfter, err = encodeFields(c.After); err != nil {
			return coreerrs.WrapField(err, "resource.changes.after")
		}
		dst.ResourceChangesFields = c.Fields
	}

	if resErr := e.Result.Error; resErr != nil {
		dst.ResultErrorCode = resErr.Code
		dst.ResultErrorMessage = resErr.Message
	}

	return nil
}

// fromRow converts a row back into an audit event. See [toRowInto] for the
// conversions that do not round-trip exactly.
//
// The free-form columns hold JSON objects; a malformed one fails the
// conversion.
func fromRow(r *eventRow) (*audit.Event, error) {
	var (
		actorMeta, attrs, meta, before, after map[string]any
		err                                   error
	)
	for _, f := range []struct {
		dst *map[string]any
		src string
	}{
		{&actorMeta, r.ActorMetadata},
		{&attrs, r.ResourceAttributes},
		{&meta, r.Metadata},
		{&before, r.ResourceChangesBefore},
		{&after, r.ResourceChangesAfter},
	} {
		if *f.dst, err = decodeFields(f.src); err != nil {
			return nil, err
		}
	}

	e := &audit.Event{
		ID:        r.ID,
		Type:      audit.EventType(r.Type),
		Action:    audit.Action(r.Action),
		Timestamp: r.Timestamp,
		Duration:  time.Duration(r.DurationNS),
		Metadata:  meta,
		Actor: audit.Actor{
			Type:      audit.ActorType(r.ActorType),
			ID:        r.ActorID,
			Name:      r.ActorName,
			Email:     r.ActorEmail,
			IP:        r.ActorIP,
			UserAgent: r.ActorUserAgent,
			Roles:     nilIfEmpty(r.ActorRoles),
			Metadata:  actorMeta,
		},
		Resource: audit.Resource{
			Type:       r.ResourceType,
			ID:         r.ResourceID,
			Name:       r.ResourceName,
			Path:       r.ResourcePath,
			Attributes: attrs,
		},
		Result: audit.Result{
			Status:  audit.ResultStatus(r.ResultStatus),
			Code:    int(r.ResultCode),
			Message: r.ResultMessage,
		},
		Context: audit.EventContext{
			RequestID:     r.RequestID,
			TraceID:       r.TraceID,
			SpanID:        r.SpanID,
			CorrelationID: r.CorrelationID,
		},
		Service: audit.ServiceInfo{
			Name:     r.ServiceName,
			Version:  r.ServiceVersion,
			Instance: r.ServiceInstance,
		},
	}

	if fields := nilIfEmpty(r.ResourceChangesFields); before != nil || after != nil || fields != nil {
		e.Resource.Changes = &audit.ResourceChanges{Before: before, After: after, Fields: fields}
	}

	if r.ResultErrorCode != "" || r.ResultErrorMessage != "" {
		e.Result.Error = &audit.ResultError{Code: r.ResultErrorCode, Message: r.ResultErrorMessage}
	}

	return e, nil
}

// scanDest returns pointers to the row fields in [schemaColumns] order,
// ready for driver.Rows.Scan.
func (r *eventRow) scanDest() []any {
	return []any{
		&r.Timestamp,
		&r.ID,
		&r.Type,
		&r.Action,

		&r.ActorType,
		&r.ActorID,
		&r.ActorName,
		&r.ActorEmail,
		&r.ActorIP,
		&r.ActorUserAgent,
		&r.ActorRoles,
		&r.ActorMetadata,

		&r.ResourceType,
		&r.ResourceID,
		&r.ResourceName,
		&r.ResourcePath,
		&r.ResourceChangesBefore,
		&r.ResourceChangesAfter,
		&r.ResourceChangesFields,
		&r.ResourceAttributes,

		&r.ResultStatus,
		&r.ResultCode,
		&r.ResultMessage,
		&r.ResultErrorCode,
		&r.ResultErrorMessage,

		&r.RequestID,
		&r.TraceID,
		&r.SpanID,
		&r.CorrelationID,

		&r.ServiceName,
		&r.ServiceVersion,
		&r.ServiceInstance,

		&r.DurationNS,
		&r.Metadata,
	}
}

// encodeFields renders a free-form map as the JSON stored in the column. An
// empty map yields the empty string, so absent data costs nothing on disk.
func encodeFields(m map[string]any) (string, error) {
	if len(m) == 0 {
		return "", nil
	}

	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

// decodeFields reverses [encodeFields]; the empty string decodes to nil.
func decodeFields(s string) (map[string]any, error) {
	if s == "" {
		return nil, nil //nolint:nilnil // nil map is the absent value
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, fmt.Errorf("decode audit column: %w", err)
	}
	if len(m) == 0 {
		return nil, nil //nolint:nilnil // nil map is the absent value
	}

	return m, nil
}

// nilIfEmpty normalizes the empty slices ClickHouse returns for empty
// Array columns back to nil, matching how [audit.Event] models absence.
func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}

	return s
}
