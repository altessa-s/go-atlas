// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/audit"
)

// eventTime is millisecond-precise, matching what DateTime64(3) can store.
var eventTime = time.Date(2026, time.March, 4, 5, 6, 7, 123_000_000, time.UTC)

func fullEvent() *audit.Event {
	return &audit.Event{
		ID:        "01JD8Z9K",
		Type:      audit.EventTypeDataChange,
		Action:    audit.ActionUpdate,
		Timestamp: eventTime,
		Duration:  1500 * time.Millisecond,
		Metadata:  map[string]any{"region": "eu-central-1"},
		Actor: audit.Actor{
			Type:      audit.ActorTypeUser,
			ID:        "user-42",
			Name:      "Ada",
			Email:     "ada@example.com",
			IP:        "203.0.113.7",
			UserAgent: "grpc-go/1.60",
			Roles:     []string{"admin", "auditor"},
			Metadata:  map[string]any{"tenant": "acme"},
		},
		Resource: audit.Resource{
			Type: "invoice",
			ID:   "inv-7",
			Name: "Invoice #7",
			Path: "/invoices/inv-7",
			Changes: &audit.ResourceChanges{
				Before: map[string]any{"total": 100.0},
				After:  map[string]any{"total": 120.0},
				Fields: []string{"total"},
			},
			Attributes: map[string]any{"currency": "EUR"},
		},
		Result: audit.Result{
			Status:  audit.ResultStatusFailure,
			Code:    409,
			Message: "conflict",
			Error:   &audit.ResultError{Code: "conflict", Message: "version mismatch"},
		},
		Context: audit.EventContext{
			RequestID:     "req-1",
			TraceID:       "trace-1",
			SpanID:        "span-1",
			CorrelationID: "corr-1",
		},
		Service: audit.ServiceInfo{Name: "billing", Version: "1.2.3", Instance: "pod-a"},
	}
}

func TestRoundTripFullEvent(t *testing.T) {
	t.Parallel()

	want := fullEvent()

	row, err := toRow(want)
	require.NoError(t, err)

	got, err := fromRow(row)
	require.NoError(t, err)
	RequireEventEqual(t, want, got)
}

func TestRoundTripZeroEvent(t *testing.T) {
	t.Parallel()

	row, err := toRow(&audit.Event{})
	require.NoError(t, err)

	got, err := fromRow(row)
	require.NoError(t, err)
	require.NoError(t, err)
	require.Equal(t, &audit.Event{}, got)
}

// Absence and presence-but-empty collapse to the same row, so the round trip
// normalizes the latter to the former. The behavior is documented on toRow.
func TestRoundTripNormalizesEmptyContainers(t *testing.T) {
	t.Parallel()

	in := &audit.Event{
		Metadata: map[string]any{},
		Actor: audit.Actor{
			Roles:    []string{},
			Metadata: map[string]any{},
		},
		Resource: audit.Resource{
			Changes:    &audit.ResourceChanges{Before: map[string]any{}, After: nil, Fields: []string{}},
			Attributes: map[string]any{},
		},
		Result: audit.Result{Error: &audit.ResultError{}},
	}

	row, err := toRow(in)
	require.NoError(t, err)

	got, err := fromRow(row)
	require.NoError(t, err)
	require.NoError(t, err)

	require.Nil(t, got.Metadata)
	require.Nil(t, got.Actor.Roles)
	require.Nil(t, got.Actor.Metadata)
	require.Nil(t, got.Resource.Attributes)
	require.Nil(t, got.Resource.Changes)
	require.Nil(t, got.Result.Error)
}

// A partially populated Changes must survive: it is only dropped when every
// part of it is empty.
func TestRoundTripKeepsPartialChanges(t *testing.T) {
	t.Parallel()

	in := &audit.Event{
		Resource: audit.Resource{
			Changes: &audit.ResourceChanges{Fields: []string{"total"}},
		},
	}

	row, err := toRow(in)
	require.NoError(t, err)

	got, err := fromRow(row)
	require.NoError(t, err)
	require.NoError(t, err)
	require.Equal(t, &audit.ResourceChanges{Fields: []string{"total"}}, got.Resource.Changes)
}

func TestToRowRejectsUnencodableMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event *audit.Event
	}{
		{
			name:  "event metadata",
			event: &audit.Event{Metadata: map[string]any{"ch": make(chan int)}},
		},
		{
			name:  "actor metadata",
			event: &audit.Event{Actor: audit.Actor{Metadata: map[string]any{"ch": make(chan int)}}},
		},
		{
			name:  "resource attributes",
			event: &audit.Event{Resource: audit.Resource{Attributes: map[string]any{"ch": make(chan int)}}},
		},
		{
			name: "resource changes",
			event: &audit.Event{Resource: audit.Resource{
				Changes: &audit.ResourceChanges{Before: map[string]any{"ch": make(chan int)}},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			row, err := toRow(tt.event)
			require.Nil(t, row)

			var typeErr *json.UnsupportedTypeError
			require.ErrorAs(t, err, &typeErr)
		})
	}
}

// A malformed JSON column fails the conversion.
func TestFromRowRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	for name, row := range map[string]*eventRow{
		"metadata":            {Metadata: "{"},
		"actor metadata":      {ActorMetadata: "{"},
		"resource attributes": {ResourceAttributes: "{"},
		"changes before":      {ResourceChangesBefore: "{"},
		"changes after":       {ResourceChangesAfter: "{"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := fromRow(row)
			var syntaxErr *json.SyntaxError
			require.ErrorAs(t, err, &syntaxErr)
		})
	}
}

// A column swap in args would silently corrupt the audit trail, so every
// string column carries its own name as its value and the whole slice is
// checked against the schema order.
func TestArgsFollowSchemaColumnOrder(t *testing.T) {
	t.Parallel()

	row := &eventRow{
		Timestamp: eventTime,
		ID:        colID,
		Type:      colType,
		Action:    colAction,

		ActorType:      colActorType,
		ActorID:        colActorID,
		ActorName:      colActorName,
		ActorEmail:     colActorEmail,
		ActorIP:        colActorIP,
		ActorUserAgent: colActorUserAgent,
		ActorRoles:     []string{colActorRoles},
		ActorMetadata:  colActorMetadata,

		ResourceType:          colResourceType,
		ResourceID:            colResourceID,
		ResourceName:          colResourceName,
		ResourcePath:          colResourcePath,
		ResourceChangesBefore: colResourceChangesBefore,
		ResourceChangesAfter:  colResourceChangesAfter,
		ResourceChangesFields: []string{colResourceChangesFields},
		ResourceAttributes:    colResourceAttributes,

		ResultStatus:       colResultStatus,
		ResultCode:         409,
		ResultMessage:      colResultMessage,
		ResultErrorCode:    colResultErrorCode,
		ResultErrorMessage: colResultErrorMessage,

		RequestID:     colRequestID,
		TraceID:       colTraceID,
		SpanID:        colSpanID,
		CorrelationID: colCorrelationID,

		ServiceName:     colServiceName,
		ServiceVersion:  colServiceVersion,
		ServiceInstance: colServiceInstance,

		DurationNS: 1500,
		Metadata:   colMetadata,
	}

	args := row.args()
	require.Len(t, args, len(schemaColumns))

	for i, c := range schemaColumns {
		switch c.name {
		case colTimestamp:
			require.Equal(t, eventTime, args[i])
		case colActorRoles, colResourceChangesFields:
			require.Equal(t, []string{c.name}, args[i], "column %s", c.name)
		case colResultCode:
			require.Equal(t, int64(409), args[i])
		case colDurationNS:
			require.Equal(t, int64(1500), args[i])
		default:
			require.Equal(t, c.name, args[i], "column %s", c.name)
		}
	}
}

// scanDest must address the same fields, in the same order, that args reads.
func TestScanDestMatchesArgs(t *testing.T) {
	t.Parallel()

	row, err := toRow(fullEvent())
	require.NoError(t, err)

	args := row.args()
	dest := row.scanDest()
	require.Len(t, dest, len(args))

	for i, c := range schemaColumns {
		ptr := reflect.ValueOf(dest[i])
		require.Equal(t, reflect.Pointer, ptr.Kind(), "column %s", c.name)
		require.Equal(t, args[i], ptr.Elem().Interface(), "column %s", c.name)
	}
}

// toRow is [toRowInto] into a fresh row.
func toRow(e *audit.Event) (*eventRow, error) {
	var r eventRow
	if err := toRowInto(&r, e); err != nil {
		return nil, err
	}

	return &r, nil
}

// args returns the column values in [schemaColumns] order, so tests can
// compare a row with what the column buffer sends.
func (r *eventRow) args() []any {
	return []any{
		r.Timestamp,
		r.ID,
		r.Type,
		r.Action,

		r.ActorType,
		r.ActorID,
		r.ActorName,
		r.ActorEmail,
		r.ActorIP,
		r.ActorUserAgent,
		r.ActorRoles,
		r.ActorMetadata,

		r.ResourceType,
		r.ResourceID,
		r.ResourceName,
		r.ResourcePath,
		r.ResourceChangesBefore,
		r.ResourceChangesAfter,
		r.ResourceChangesFields,
		r.ResourceAttributes,

		r.ResultStatus,
		r.ResultCode,
		r.ResultMessage,
		r.ResultErrorCode,
		r.ResultErrorMessage,

		r.RequestID,
		r.TraceID,
		r.SpanID,
		r.CorrelationID,

		r.ServiceName,
		r.ServiceVersion,
		r.ServiceInstance,

		r.DurationNS,
		r.Metadata,
	}
}
