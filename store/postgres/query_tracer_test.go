// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/sampler"
)

func TestQueryTracerSampledParent(t *testing.T) {
	t.Parallel()
	adapter := new(spanCollector)
	tracer := tracing.New(tracing.WithAdapter(adapter))
	t.Cleanup(func() { _ = tracer.Shutdown(context.Background()) })
	query := NewQueryTracer(tracer)
	ctx, parent := tracer.Recorder("test").Start(t.Context(), "parent")
	defer parent.End()
	conn := queryTestConnection(t)
	ctx = query.TraceQueryStart(ctx, conn, pgx.TraceQueryStartData{
		SQL: "-- name: FindKey :one\n SELECT   key\n FROM keys WHERE id = $1", Args: []any{"private"},
	})
	query.TraceQueryEnd(ctx, conn, pgx.TraceQueryEndData{CommandTag: pgconn.NewCommandTag("SELECT 1")})

	require.NoError(t, tracer.ForceFlush(t.Context()))
	spans := adapter.Spans()
	require.Len(t, spans, 1)
	span := spans[0]
	require.Equal(t, "FindKey", span.Name)
	require.Equal(t, adapters.SpanKindClient, span.Kind)
	require.Equal(t, parent.SpanContext().TraceID(), span.TraceID.String())
	require.Equal(t, parent.SpanContext().SpanID(), span.ParentID.String())
	require.ElementsMatch(t, []adapters.Attribute{
		{Key: "db.system.name", Value: "postgresql"},
		{Key: "db.namespace", Value: "db"},
		{Key: "db.operation.name", Value: "SELECT"},
		{Key: "db.query.summary", Value: "FindKey"},
		{Key: "db.query.text", Value: "SELECT key FROM keys WHERE id = $1"},
		{Key: "server.address", Value: "127.0.0.1"},
		{Key: "server.port", Value: 5432},
		{Key: "db.rows_affected", Value: int64(1)},
	}, span.Attributes)
}

func TestQueryTracerWithoutRecordingParent(t *testing.T) {
	t.Parallel()
	adapter := new(spanCollector)
	tracer := tracing.New(tracing.WithAdapter(adapter))
	t.Cleanup(func() { _ = tracer.Shutdown(context.Background()) })
	unsampledTracer := tracing.New(tracing.WithAdapter(adapter), tracing.WithSampler(sampler.AlwaysOff()))
	unsampled, parent := unsampledTracer.Recorder("test").Start(t.Context(), "unsampled")
	defer parent.End()
	defer func() { _ = unsampledTracer.Shutdown(context.Background()) }()
	remote := tracing.NewRemoteSpanContext([16]byte{1}, [8]byte{1}, tracing.FlagsSampled)
	query := NewQueryTracer(tracer)
	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{"no parent", t.Context()},
		{"unsampled parent", unsampled},
		{"sampled remote context", tracing.ContextWithSpanContext(t.Context(), remote)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := query.TraceQueryStart(tc.ctx, nil, pgx.TraceQueryStartData{SQL: "SELECT $1", Args: []any{"private"}})
			require.Same(t, tc.ctx, ctx)
			query.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{CommandTag: pgconn.NewCommandTag("SELECT 1")})
			require.Empty(t, adapter.Spans())
		})
	}
}

func TestQueryTracerError(t *testing.T) {
	t.Parallel()
	adapter := new(spanCollector)
	tracer := tracing.New(tracing.WithAdapter(adapter))
	t.Cleanup(func() { _ = tracer.Shutdown(context.Background()) })
	query := NewQueryTracer(tracer)
	ctx, parent := tracer.Recorder("test").Start(t.Context(), "parent")
	defer parent.End()
	conn := queryTestConnection(t)
	ctx = query.TraceQueryStart(ctx, conn, pgx.TraceQueryStartData{SQL: "select $1::uuid"})
	err := fmt.Errorf("read key: %w", &pgconn.PgError{Code: "22P02", Message: "invalid UUID: private parameter"})
	query.TraceQueryEnd(ctx, conn, pgx.TraceQueryEndData{Err: err})

	require.NoError(t, tracer.ForceFlush(t.Context()))
	spans := adapter.Spans()
	require.Len(t, spans, 1)
	span := spans[0]
	require.Equal(t, "SELECT", span.Name)
	require.Equal(t, adapters.StatusError, span.Status)
	require.Contains(t, span.Attributes, adapters.Attribute{Key: "db.response.status_code", Value: "22P02"})
	require.Len(t, span.Events, 1)
	require.Equal(t, "exception", span.Events[0].Name)
	require.NotContains(t, fmt.Sprint(span), "private parameter")
}

func TestQueryDetails(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		title, sql, name, operation, text string
	}{
		{"sqlc", "-- name: UpsertKey :execrows\n INSERT\t INTO keys VALUES ($1)", "UpsertKey", "INSERT", "INSERT INTO keys VALUES ($1)"},
		{"sqlc with cte", "\n -- name: Claim :many\r\nWITH due AS (SELECT 1) SELECT * FROM due", "Claim", "WITH", "WITH due AS (SELECT 1) SELECT * FROM due"},
		{"lowercase", "  begin;\n SET LOCAL lock_timeout = 1", "BEGIN", "BEGIN", "begin; SET LOCAL lock_timeout = 1"},
		{"commit", "COMMIT", "COMMIT", "COMMIT", "COMMIT"},
		{"leading comments", "/* lock */ -- more\nselect(1)", "SELECT", "SELECT", "/* lock */ -- more select(1)"},
		{"malformed header", "-- name: Invalid\nselect 1", "SELECT", "SELECT", "-- name: Invalid select 1"},
		{"empty", " \n", "SQL", "SQL", ""},
	} {
		t.Run(tc.title, func(t *testing.T) {
			name, operation, sql := queryDetails(tc.sql)
			require.Equal(t, tc.name, name)
			require.Equal(t, tc.operation, operation)
			require.Equal(t, tc.text, queryText(sql))
		})
	}
	long := queryText("SELECT " + strings.Repeat("日", 2048))
	require.LessOrEqual(t, len(long), 2048)
	require.True(t, utf8.ValidString(long))
	require.Equal(t, 2048, len(queryText("SELECT "+strings.Repeat("x", 2048))))
}

func queryTestConnection(t *testing.T) *pgx.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	return statementErrorConnection(ctx, t, PoolingDirect).Conn()
}

type spanCollector struct {
	adapters.MultiAdapter
	spans []adapters.SpanData
}

func (a *spanCollector) ExportSpans(_ context.Context, spans []adapters.SpanData) (adapters.ExportResult, error) {
	a.spans = append(a.spans, spans...)
	return adapters.ExportResult{Accepted: len(spans)}, nil
}
func (a *spanCollector) Spans() []adapters.SpanData { return a.spans }
