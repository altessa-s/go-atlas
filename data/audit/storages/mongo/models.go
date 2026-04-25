// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"time"

	"github.com/altessa-s/go-atlas/data/audit"
)

// eventModel is the BSON representation of an audit event.
type eventModel struct {
	ID        string            `bson:"_id"`
	Type      string            `bson:"type"`
	Action    string            `bson:"action"`
	Actor     actorModel        `bson:"actor"`
	Resource  resourceModel     `bson:"resource"`
	Result    resultModel       `bson:"result"`
	Context   eventContextModel `bson:"context"`
	Service   serviceInfoModel  `bson:"service"`
	Timestamp time.Time         `bson:"timestamp"`
	Duration  int64             `bson:"duration,omitempty"`
	Metadata  map[string]any    `bson:"metadata,omitempty"`
}

type actorModel struct {
	Type      string         `bson:"type"`
	ID        string         `bson:"id"`
	Name      string         `bson:"name,omitempty"`
	Email     string         `bson:"email,omitempty"`
	IP        string         `bson:"ip,omitempty"`
	UserAgent string         `bson:"user_agent,omitempty"`
	Roles     []string       `bson:"roles,omitempty"`
	Metadata  map[string]any `bson:"metadata,omitempty"`
}

type resourceModel struct {
	Type       string                `bson:"type"`
	ID         string                `bson:"id"`
	Name       string                `bson:"name,omitempty"`
	Path       string                `bson:"path,omitempty"`
	Changes    *resourceChangesModel `bson:"changes,omitempty"`
	Attributes map[string]any        `bson:"attributes,omitempty"`
}

type resourceChangesModel struct {
	Before map[string]any `bson:"before,omitempty"`
	After  map[string]any `bson:"after,omitempty"`
	Fields []string       `bson:"fields,omitempty"`
}

type resultModel struct {
	Status  string            `bson:"status"`
	Code    int               `bson:"code,omitempty"`
	Message string            `bson:"message,omitempty"`
	Error   *resultErrorModel `bson:"error,omitempty"`
}

type resultErrorModel struct {
	Code    string `bson:"code,omitempty"`
	Message string `bson:"message,omitempty"`
}

type eventContextModel struct {
	RequestID     string `bson:"request_id,omitempty"`
	TraceID       string `bson:"trace_id,omitempty"`
	SpanID        string `bson:"span_id,omitempty"`
	CorrelationID string `bson:"correlation_id,omitempty"`
}

type serviceInfoModel struct {
	Name     string `bson:"name"`
	Version  string `bson:"version,omitempty"`
	Instance string `bson:"instance,omitempty"`
}

func toModel(e *audit.Event) *eventModel {
	m := &eventModel{
		ID:        e.ID,
		Type:      string(e.Type),
		Action:    string(e.Action),
		Timestamp: e.Timestamp,
		Duration:  int64(e.Duration),
		Metadata:  e.Metadata,
		Actor: actorModel{
			Type:      string(e.Actor.Type),
			ID:        e.Actor.ID,
			Name:      e.Actor.Name,
			Email:     e.Actor.Email,
			IP:        e.Actor.IP,
			UserAgent: e.Actor.UserAgent,
			Roles:     e.Actor.Roles,
			Metadata:  e.Actor.Metadata,
		},
		Resource: resourceModel{
			Type:       e.Resource.Type,
			ID:         e.Resource.ID,
			Name:       e.Resource.Name,
			Path:       e.Resource.Path,
			Attributes: e.Resource.Attributes,
		},
		Result: resultModel{
			Status:  string(e.Result.Status),
			Code:    e.Result.Code,
			Message: e.Result.Message,
		},
		Context: eventContextModel{
			RequestID:     e.Context.RequestID,
			TraceID:       e.Context.TraceID,
			SpanID:        e.Context.SpanID,
			CorrelationID: e.Context.CorrelationID,
		},
		Service: serviceInfoModel{
			Name:     e.Service.Name,
			Version:  e.Service.Version,
			Instance: e.Service.Instance,
		},
	}

	if e.Resource.Changes != nil {
		m.Resource.Changes = &resourceChangesModel{
			Before: e.Resource.Changes.Before,
			After:  e.Resource.Changes.After,
			Fields: e.Resource.Changes.Fields,
		}
	}

	if e.Result.Error != nil {
		m.Result.Error = &resultErrorModel{
			Code:    e.Result.Error.Code,
			Message: e.Result.Error.Message,
		}
	}

	return m
}

func fromModel(m *eventModel) *audit.Event {
	e := &audit.Event{
		ID:        m.ID,
		Type:      audit.EventType(m.Type),
		Action:    audit.Action(m.Action),
		Timestamp: m.Timestamp,
		Duration:  time.Duration(m.Duration),
		Metadata:  m.Metadata,
		Actor: audit.Actor{
			Type:      audit.ActorType(m.Actor.Type),
			ID:        m.Actor.ID,
			Name:      m.Actor.Name,
			Email:     m.Actor.Email,
			IP:        m.Actor.IP,
			UserAgent: m.Actor.UserAgent,
			Roles:     m.Actor.Roles,
			Metadata:  m.Actor.Metadata,
		},
		Resource: audit.Resource{
			Type:       m.Resource.Type,
			ID:         m.Resource.ID,
			Name:       m.Resource.Name,
			Path:       m.Resource.Path,
			Attributes: m.Resource.Attributes,
		},
		Result: audit.Result{
			Status:  audit.ResultStatus(m.Result.Status),
			Code:    m.Result.Code,
			Message: m.Result.Message,
		},
		Context: audit.EventContext{
			RequestID:     m.Context.RequestID,
			TraceID:       m.Context.TraceID,
			SpanID:        m.Context.SpanID,
			CorrelationID: m.Context.CorrelationID,
		},
		Service: audit.ServiceInfo{
			Name:     m.Service.Name,
			Version:  m.Service.Version,
			Instance: m.Service.Instance,
		},
	}

	if m.Resource.Changes != nil {
		e.Resource.Changes = &audit.ResourceChanges{
			Before: m.Resource.Changes.Before,
			After:  m.Resource.Changes.After,
			Fields: m.Resource.Changes.Fields,
		}
	}

	if m.Result.Error != nil {
		e.Result.Error = &audit.ResultError{
			Code:    m.Result.Error.Code,
			Message: m.Result.Error.Message,
		}
	}

	return e
}
