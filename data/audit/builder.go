// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

// EventBuilder provides a fluent API for constructing audit events.
type EventBuilder struct {
	auditor *Auditor
	event   *Event
}

// WithActor sets the actor.
func (b *EventBuilder) WithActor(actor Actor) *EventBuilder {
	b.event.Actor = actor
	return b
}

// WithResource sets the resource.
func (b *EventBuilder) WithResource(resource Resource) *EventBuilder {
	b.event.Resource = resource
	return b
}

// WithResult sets the result.
func (b *EventBuilder) WithResult(result Result) *EventBuilder {
	b.event.Result = result
	return b
}

// WithSuccess sets the result to success.
func (b *EventBuilder) WithSuccess() *EventBuilder {
	b.event.Result = Result{Status: ResultStatusSuccess}
	return b
}

// WithFailure sets the result to failure with an optional message.
func (b *EventBuilder) WithFailure(code int, message string) *EventBuilder {
	b.event.Result = Result{Status: ResultStatusFailure, Code: code, Message: message}
	return b
}

// WithError sets the result to error with details.
func (b *EventBuilder) WithError(err error) *EventBuilder {
	b.event.Result = Result{
		Status: ResultStatusError,
		Error:  &ResultError{Message: err.Error()},
	}
	return b
}

// WithEventContext sets correlation identifiers.
func (b *EventBuilder) WithEventContext(ec EventContext) *EventBuilder {
	b.event.Context = ec
	return b
}

// WithChanges sets the resource change details.
func (b *EventBuilder) WithChanges(before, after map[string]any, fields []string) *EventBuilder {
	b.event.Resource.Changes = &ResourceChanges{
		Before: before,
		After:  after,
		Fields: fields,
	}
	return b
}

// WithMetadata sets additional metadata.
func (b *EventBuilder) WithMetadata(metadata map[string]any) *EventBuilder {
	b.event.Metadata = metadata
	return b
}

// Build returns the constructed event without emitting it.
func (b *EventBuilder) Build() *Event {
	return b.event
}

// Emit sends the constructed event to the auditor.
func (b *EventBuilder) Emit() bool {
	return b.auditor.Emit(b.event)
}
