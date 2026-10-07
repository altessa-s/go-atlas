// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// columnBuffer accumulates a batch column-wise so it can be handed to the
// driver one whole column at a time.
//
// The row-oriented alternative, driver.Batch.Append(values...), takes the
// values as []any: every string, int64, and time.Time is boxed into an
// interface, which for a non-pointer-shaped value means a heap allocation.
// At 34 columns that is ~35 allocations per row. Appending columns instead
// makes the cost O(columns) per batch rather than O(columns × rows).
type columnBuffer struct {
	timestamp []time.Time
	id        []string
	eventType []string
	action    []string

	actorType      []string
	actorID        []string
	actorName      []string
	actorEmail     []string
	actorIP        []string
	actorUserAgent []string
	actorRoles     [][]string
	actorMetadata  []string

	resourceType          []string
	resourceID            []string
	resourceName          []string
	resourcePath          []string
	resourceChangesBefore []string
	resourceChangesAfter  []string
	resourceChangesFields [][]string
	resourceAttributes    []string

	resultStatus       []string
	resultCode         []int64
	resultMessage      []string
	resultErrorCode    []string
	resultErrorMessage []string

	requestID     []string
	traceID       []string
	spanID        []string
	correlationID []string

	serviceName     []string
	serviceVersion  []string
	serviceInstance []string

	durationNS []int64
	metadata   []string
}

// newColumnBuffer returns a buffer sized for a batch of n rows.
func newColumnBuffer(n int) *columnBuffer {
	return &columnBuffer{
		timestamp: make([]time.Time, 0, n),
		id:        make([]string, 0, n),
		eventType: make([]string, 0, n),
		action:    make([]string, 0, n),

		actorType:      make([]string, 0, n),
		actorID:        make([]string, 0, n),
		actorName:      make([]string, 0, n),
		actorEmail:     make([]string, 0, n),
		actorIP:        make([]string, 0, n),
		actorUserAgent: make([]string, 0, n),
		actorRoles:     make([][]string, 0, n),
		actorMetadata:  make([]string, 0, n),

		resourceType:          make([]string, 0, n),
		resourceID:            make([]string, 0, n),
		resourceName:          make([]string, 0, n),
		resourcePath:          make([]string, 0, n),
		resourceChangesBefore: make([]string, 0, n),
		resourceChangesAfter:  make([]string, 0, n),
		resourceChangesFields: make([][]string, 0, n),
		resourceAttributes:    make([]string, 0, n),

		resultStatus:       make([]string, 0, n),
		resultCode:         make([]int64, 0, n),
		resultMessage:      make([]string, 0, n),
		resultErrorCode:    make([]string, 0, n),
		resultErrorMessage: make([]string, 0, n),

		requestID:     make([]string, 0, n),
		traceID:       make([]string, 0, n),
		spanID:        make([]string, 0, n),
		correlationID: make([]string, 0, n),

		serviceName:     make([]string, 0, n),
		serviceVersion:  make([]string, 0, n),
		serviceInstance: make([]string, 0, n),

		durationNS: make([]int64, 0, n),
		metadata:   make([]string, 0, n),
	}
}

// appendRow copies one row into the per-column slices.
func (c *columnBuffer) appendRow(r *eventRow) {
	c.timestamp = append(c.timestamp, r.Timestamp)
	c.id = append(c.id, r.ID)
	c.eventType = append(c.eventType, r.Type)
	c.action = append(c.action, r.Action)

	c.actorType = append(c.actorType, r.ActorType)
	c.actorID = append(c.actorID, r.ActorID)
	c.actorName = append(c.actorName, r.ActorName)
	c.actorEmail = append(c.actorEmail, r.ActorEmail)
	c.actorIP = append(c.actorIP, r.ActorIP)
	c.actorUserAgent = append(c.actorUserAgent, r.ActorUserAgent)
	c.actorRoles = append(c.actorRoles, r.ActorRoles)
	c.actorMetadata = append(c.actorMetadata, r.ActorMetadata)

	c.resourceType = append(c.resourceType, r.ResourceType)
	c.resourceID = append(c.resourceID, r.ResourceID)
	c.resourceName = append(c.resourceName, r.ResourceName)
	c.resourcePath = append(c.resourcePath, r.ResourcePath)
	c.resourceChangesBefore = append(c.resourceChangesBefore, r.ResourceChangesBefore)
	c.resourceChangesAfter = append(c.resourceChangesAfter, r.ResourceChangesAfter)
	c.resourceChangesFields = append(c.resourceChangesFields, r.ResourceChangesFields)
	c.resourceAttributes = append(c.resourceAttributes, r.ResourceAttributes)

	c.resultStatus = append(c.resultStatus, r.ResultStatus)
	c.resultCode = append(c.resultCode, r.ResultCode)
	c.resultMessage = append(c.resultMessage, r.ResultMessage)
	c.resultErrorCode = append(c.resultErrorCode, r.ResultErrorCode)
	c.resultErrorMessage = append(c.resultErrorMessage, r.ResultErrorMessage)

	c.requestID = append(c.requestID, r.RequestID)
	c.traceID = append(c.traceID, r.TraceID)
	c.spanID = append(c.spanID, r.SpanID)
	c.correlationID = append(c.correlationID, r.CorrelationID)

	c.serviceName = append(c.serviceName, r.ServiceName)
	c.serviceVersion = append(c.serviceVersion, r.ServiceVersion)
	c.serviceInstance = append(c.serviceInstance, r.ServiceInstance)

	c.durationNS = append(c.durationNS, r.DurationNS)
	c.metadata = append(c.metadata, r.Metadata)
}

// values returns the column slices in [schemaColumns] order. The order is
// the contract with the INSERT statement; the tests pin it.
func (c *columnBuffer) values() []any {
	return []any{
		c.timestamp,
		c.id,
		c.eventType,
		c.action,

		c.actorType,
		c.actorID,
		c.actorName,
		c.actorEmail,
		c.actorIP,
		c.actorUserAgent,
		c.actorRoles,
		c.actorMetadata,

		c.resourceType,
		c.resourceID,
		c.resourceName,
		c.resourcePath,
		c.resourceChangesBefore,
		c.resourceChangesAfter,
		c.resourceChangesFields,
		c.resourceAttributes,

		c.resultStatus,
		c.resultCode,
		c.resultMessage,
		c.resultErrorCode,
		c.resultErrorMessage,

		c.requestID,
		c.traceID,
		c.spanID,
		c.correlationID,

		c.serviceName,
		c.serviceVersion,
		c.serviceInstance,

		c.durationNS,
		c.metadata,
	}
}

// flush hands every column to the batch.
func (c *columnBuffer) flush(batch driver.Batch) error {
	for i, values := range c.values() {
		if err := batch.Column(i).Append(values); err != nil {
			return coreerrs.WrapField(err, schemaColumns[i].name)
		}
	}

	return nil
}
