// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"encoding/json"
)

// DefaultMaxReplaySize bounds a response stored for replay, in bytes (the
// serialized message for gRPC, the body for HTTP). A larger response is not
// stored: a repeat of its request is refused as already used.
const DefaultMaxReplaySize = 256 << 10

// Record is the data a transport stores with a completed key when response
// replay is on. Its JSON form is persisted in the idempotency storage, so the
// field names are frozen. A record written without replay is the bare entity
// ID string instead (see [DecodeRecord]).
type Record struct {
	// EntityID is the entity ID extracted from the response, "" for none.
	EntityID string `json:"entity_id,omitempty"`
	// Binding names the operation the response belongs to — the full gRPC
	// method ("/package.Service/Method"), or the HTTP method and path
	// ("POST /orders"). gRPC storage keys are scoped by service only, so a
	// response is replayed only to a request with the same binding; HTTP
	// keys are already METHOD:path scoped, and the binding guards the
	// replay the same way. A record without one is never replayed.
	Binding string `json:"binding,omitempty"`
	// Response is the gRPC response, a marshaled google.protobuf.Any.
	Response []byte `json:"response,omitempty"`
	// Status is the HTTP status code of the response; 0 when the record
	// holds no HTTP response.
	Status int `json:"status,omitempty"`
	// Header holds the replayed HTTP response headers.
	Header map[string][]string `json:"header,omitempty"`
	// Body is the HTTP response body.
	Body []byte `json:"body,omitempty"`
}

// EntityIDData returns the data a transport stores with a completed key
// when response replay is off: entityID, or nil when it is empty — the
// format written before replay existed, which [DecodeRecord] reads back.
func EntityIDData(entityID string) any {
	if entityID == "" {
		return nil
	}
	return entityID
}

// NewRecord returns the base record a transport stores with a completed key
// when response replay is on: entityID and the binding of the request. The
// transport attaches its response before storing it.
func NewRecord(entityID, binding string) Record {
	return Record{EntityID: entityID, Binding: binding}
}

// HasGRPCResponse reports whether r holds a gRPC response to replay for
// binding.
func (r Record) HasGRPCResponse(binding string) bool {
	return len(r.Response) > 0 && r.Binding != "" && r.Binding == binding
}

// HasHTTPResponse reports whether r holds an HTTP response to replay for
// binding.
func (r Record) HasHTTPResponse(binding string) bool {
	return r.Status != 0 && r.Binding != "" && r.Binding == binding
}

// DecodeRecord reads the data stored with a completed key: a string is the
// entity ID of a record written without replay; any other value — the
// Keeper's serializer decodes the stored JSON object into a map — is decoded
// into a Record through its JSON form. It reports false for nil or a value
// that is not a record.
func DecodeRecord(data any) (Record, bool) {
	switch v := data.(type) {
	case nil:
		return Record{}, false
	case string:
		return Record{EntityID: v}, v != ""
	case Record:
		return v, true
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return Record{}, false
	}
	var r Record
	if err := json.Unmarshal(raw, &r); err != nil {
		return Record{}, false
	}
	return r, true
}
