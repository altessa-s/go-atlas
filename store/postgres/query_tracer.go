// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/observability/tracing"
)

var _ pgx.QueryTracer = (*QueryTracer)(nil)

// QueryTracer records pgx queries as children of sampled, recording spans.
type QueryTracer struct {
	recorder tracing.Recorder
}

// NewQueryTracer creates a PostgreSQL query tracer. A nil or no-op tracer disables instrumentation.
func NewQueryTracer(tracer tracing.Tracer) *QueryTracer {
	t := new(QueryTracer)
	if !nilcheck.IsNil(tracer) && !tracing.IsNoop(tracer) {
		t.recorder = tracer.Recorder("postgresql")
	}
	return t
}

// TraceQueryStart starts a client span for a query under a sampled parent.
func (t *QueryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if t.recorder == nil {
		return ctx
	}
	parent := tracing.SpanFromContext(ctx)
	if !parent.IsRecording() {
		return ctx
	}
	if sc := parent.SpanContext(); sc == nil || !sc.IsValid() || !sc.IsSampled() {
		return ctx
	}
	name, operation, statement := queryDetails(data.SQL)
	_, span := t.recorder.Start(ctx, name, tracing.WithSpanKind(tracing.SpanKindClient))
	if !span.IsRecording() {
		return ctx
	}
	cfg := conn.Config()
	span.SetAttributes(
		tracing.String("db.system.name", "postgresql"),
		tracing.String("db.namespace", cfg.Database),
		tracing.String("db.operation.name", operation),
		tracing.String("db.query.summary", name),
		tracing.String("db.query.text", queryText(statement)),
		tracing.String("server.address", cfg.Host),
		tracing.Int("server.port", int(cfg.Port)),
	)
	return context.WithValue(ctx, querySpanKey{}, span)
}

// TraceQueryEnd records the query result and ends its client span.
func (*QueryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span, ok := ctx.Value(querySpanKey{}).(tracing.Span)
	if !ok {
		return
	}
	if tag := data.CommandTag.String(); tag != "" && tag[len(tag)-1] >= '0' && tag[len(tag)-1] <= '9' {
		span.SetAttributes(tracing.Int64("db.rows_affected", data.CommandTag.RowsAffected()))
	}
	if data.Err != nil {
		const message = "PostgreSQL query failed"
		if pgErr, ok := errors.AsType[*pgconn.PgError](data.Err); ok {
			span.SetAttributes(tracing.String("db.response.status_code", pgErr.Code))
		}
		// Server and driver error messages can echo bound parameter values.
		span.AddEvent("exception", tracing.WithEventAttributes(
			tracing.String("exception.type", fmt.Sprintf("%T", data.Err)),
			tracing.String("exception.message", message),
		))
		span.SetStatus(tracing.StatusError, message)
	}
	span.End()
}

type querySpanKey struct{}

func queryDetails(sql string) (name, operation, statement string) {
	statement = strings.TrimSpace(sql)
	line, body, _ := strings.Cut(statement, "\n")
	if header, ok := strings.CutPrefix(line, "-- name:"); ok {
		fields := strings.Fields(header)
		if len(fields) == 2 && len(fields[1]) > 1 && fields[1][0] == ':' {
			name, statement = fields[0], strings.TrimSpace(body)
		}
	}
	operation = queryOperation(statement)
	if name == "" {
		name = operation
	}
	return name, operation, statement
}

func queryOperation(sql string) string {
	for {
		sql = strings.TrimSpace(sql)
		switch {
		case strings.HasPrefix(sql, "--"):
			_, sql, _ = strings.Cut(sql, "\n")
		case strings.HasPrefix(sql, "/*"):
			_, sql, _ = strings.Cut(sql[2:], "*/")
		default:
			end := 0
			for end < len(sql) && (sql[end] >= 'a' && sql[end] <= 'z' || sql[end] >= 'A' && sql[end] <= 'Z') {
				end++
			}
			if end == 0 {
				return "SQL"
			}
			return strings.ToUpper(sql[:end])
		}
	}
}

func queryText(sql string) string {
	const maxBytes = 2 * 1024
	var text strings.Builder
	for field := range strings.FieldsSeq(sql) {
		space := 0
		if text.Len() > 0 {
			space = 1
		}
		n := maxBytes - text.Len() - space
		if n <= 0 {
			break
		}
		n = min(n, len(field))
		for n < len(field) && !utf8.RuneStart(field[n]) {
			n--
		}
		if n == 0 {
			break
		}
		if space != 0 {
			text.WriteByte(' ')
		}
		text.WriteString(field[:n])
		if n < len(field) {
			break
		}
	}
	return text.String()
}
