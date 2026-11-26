// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package explain

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/internal/diagnostics"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// HighExaminationRatio is the warning threshold for examined versus returned documents.
const HighExaminationRatio = 10

const (
	stageUnknown  = "UNKNOWN"
	stageCollscan = "COLLSCAN"
)

// Stats contains key performance metrics from MongoDB explain output.
type Stats struct {
	// IndexName is the name of the index used, empty if no index used.
	IndexName string `json:"index_name,omitempty"`
	// Stage is the winning execution plan stage (e.g., "IXSCAN", "COLLSCAN").
	Stage string `json:"stage"`
	// DocsExamined is the number of documents examined during execution.
	DocsExamined int64 `json:"docs_examined"`
	// DocsReturned is the number of documents returned by the query.
	DocsReturned int64 `json:"docs_returned"`
	// ExecutionTimeMillis is the total execution time in milliseconds.
	ExecutionTimeMillis int64 `json:"execution_time_millis"`
	// KeysExamined is the number of index keys examined.
	KeysExamined int64 `json:"keys_examined,omitempty"`
	// IndexUsed indicates whether an index was used (true) or not (false).
	IndexUsed bool `json:"index_used"`
	// IsMultiKey indicates if the index used is multikey (can impact performance).
	IsMultiKey bool `json:"is_multi_key,omitempty"`
}

// Run executes an aggregation pipeline explain command for query plan analysis.
// By default it uses [VerbosityQueryPlanner] so the pipeline is not executed a
// second time; pass [WithVerbosity] with [VerbosityExecutionStats] to collect
// runtime counters. An unknown verbosity returns an invalid argument error
// during command construction. [WithInsecureLogRawExplain] enables raw result
// logging with the contents of $literal expressions redacted.
func Run(ctx context.Context, collection *mongo.Collection, pipeline bson.A, hint any, logger *slog.Logger, opts ...Option) (*Stats, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	o := newOptions(opts...)
	switch o.verbosity {
	case VerbosityQueryPlanner, VerbosityExecutionStats:
	default:
		return nil, coreerrs.WrapOperation(coreerrs.ErrInvalidArgument, "invalid MongoDB explain verbosity")
	}

	innerCmd := bson.D{
		{Key: "aggregate", Value: collection.Name()},
		{Key: "pipeline", Value: pipeline},
		{Key: "cursor", Value: bson.D{}},
	}
	if hint != nil {
		innerCmd = append(innerCmd, bson.E{Key: "hint", Value: hint})
	}
	explainCmd := bson.D{
		{Key: "explain", Value: innerCmd},
		{Key: "verbosity", Value: string(o.verbosity)},
	}

	stages, fields := summarizePipeline(pipeline)

	var explainResult bson.M
	err := collection.Database().RunCommand(ctx, explainCmd).Decode(&explainResult)
	if err != nil {
		attrs := []any{
			slog.String("collection", collection.Name()),
			slog.Any("stages", stages),
			slog.Any("fields", fields),
			slog.String("error_class", diagnostics.ErrorClass(err)),
		}
		if cmdErr, ok := errors.AsType[mongo.CommandError](err); ok {
			attrs = append(attrs,
				slog.Int64("error_code", int64(cmdErr.Code)),
				slog.String("error_name", cmdErr.Name),
			)
		}
		logger.ErrorContext(ctx, "failed to execute explain command", attrs...)
		return nil, err
	}

	stats := parseExplainResult(explainResult)
	if o.insecureLogRawExplain && logger.Enabled(ctx, slog.LevelDebug) {
		redactExplainLiterals(explainResult)
		logger.DebugContext(ctx, "raw explain result", "explain_output", explainResult)
	}

	logStats(ctx, stats, collection.Name(), stages, fields, logger)

	return stats, nil
}

func summarizePipeline(pipeline bson.A) ([]string, []string) {
	var stages []string
	var fields []string
	for _, elem := range pipeline {
		doc, ok := asDoc(elem)
		if !ok {
			continue
		}
		for k, v := range doc {
			stages = append(stages, k)
			if k == "$match" {
				collectFilterFields(v, &fields)
			}
		}
	}
	return stages, fields
}

func collectFilterFields(v any, fields *[]string) {
	doc, ok := asDoc(v)
	if !ok {
		return
	}
	for k, sub := range doc {
		switch k {
		case "$and", "$or", "$nor":
			if arr, ok := sub.(bson.A); ok {
				for _, item := range arr {
					collectFilterFields(item, fields)
				}
			}
		default:
			// Other operators can contain expressions or data. In particular,
			// every key and value inside a $literal expression is opaque.
			if !strings.HasPrefix(k, "$") && !slices.Contains(*fields, k) {
				*fields = append(*fields, k)
			}
		}
	}
}

// redactExplainLiterals modifies the decoded result after statistics have been
// read. Both document layouts can occur in values decoded into an interface.
func redactExplainLiterals(v any) {
	const literal = "$literal"
	const redacted = "[REDACTED]"
	switch d := v.(type) {
	case bson.M:
		for k, value := range d {
			if k == literal {
				d[k] = redacted
			} else {
				redactExplainLiterals(value)
			}
		}
	case bson.D:
		for i, e := range d {
			if e.Key == literal {
				d[i].Value = redacted
			} else {
				redactExplainLiterals(e.Value)
			}
		}
	case bson.A:
		for _, item := range d {
			redactExplainLiterals(item)
		}
	}
}

func asDoc(v any) (bson.M, bool) {
	switch d := v.(type) {
	case bson.M:
		return d, true
	case map[string]any:
		return bson.M(d), true
	case bson.D:
		m := make(bson.M, len(d))
		for _, e := range d {
			m[e.Key] = e.Value
		}
		return m, true
	default:
		return nil, false
	}
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int32:
		return int64(n), true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

func parseExplainResult(explainResult bson.M) *Stats {
	stats := &Stats{
		Stage:     stageUnknown,
		IndexUsed: false,
	}

	parseExplainContainer(explainResult, stats)

	if stages, ok := explainResult["stages"].(bson.A); ok {
		for _, item := range stages {
			stageDoc, ok := asDoc(item)
			if !ok {
				continue
			}
			if cursor, ok := asDoc(stageDoc["$cursor"]); ok {
				parseExplainContainer(cursor, stats)
				break
			}
		}
	}

	return stats
}

func parseExplainContainer(doc bson.M, stats *Stats) {
	if queryPlanner, ok := asDoc(doc["queryPlanner"]); ok {
		if winningPlan, ok := asDoc(queryPlanner["winningPlan"]); ok {
			parseWinningPlan(winningPlan, stats)
		}
	}
	if executionStats, ok := asDoc(doc["executionStats"]); ok {
		parseExecutionStats(executionStats, stats)
	}
}

func parseExecutionStats(executionStats bson.M, stats *Stats) {
	if n, ok := asInt64(executionStats["totalDocsExamined"]); ok {
		stats.DocsExamined = n
	}
	if n, ok := asInt64(executionStats["nReturned"]); ok {
		stats.DocsReturned = n
	}
	if n, ok := asInt64(executionStats["executionTimeMillis"]); ok {
		stats.ExecutionTimeMillis = n
	}
	if n, ok := asInt64(executionStats["totalKeysExamined"]); ok && stats.KeysExamined == 0 {
		stats.KeysExamined = n
	}

	if stats.Stage == stageUnknown {
		if winningPlan, ok := asDoc(executionStats["winningPlan"]); ok {
			parseWinningPlan(winningPlan, stats)
		} else if executionStages, ok := asDoc(executionStats["executionStages"]); ok {
			parseWinningPlan(executionStages, stats)
		}
	}
}

func parseWinningPlan(winningPlan bson.M, stats *Stats) {
	if queryPlan, ok := asDoc(winningPlan["queryPlan"]); ok {
		parseWinningPlan(queryPlan, stats)
		if stats.Stage != stageUnknown {
			return
		}
	}

	stage, hasStage := winningPlan["stage"].(string)
	if hasStage {
		stats.Stage = stage
	}

	switch stage {
	case "IXSCAN", "EXPRESS_IXSCAN", "DISTINCT_SCAN", "COUNT_SCAN":
		stats.IndexUsed = true
		if indexName, ok := winningPlan["indexName"].(string); ok {
			stats.IndexName = indexName
		}
		if keysExamined, ok := asInt64(winningPlan["keysExamined"]); ok {
			stats.KeysExamined = keysExamined
		}
		if isMultiKey, ok := winningPlan["isMultiKey"].(bool); ok {
			stats.IsMultiKey = isMultiKey
		}
		return
	case "IDHACK":
		stats.IndexUsed = true
		if stats.IndexName == "" {
			stats.IndexName = "_id_"
		}
		return
	case stageCollscan:
		stats.IndexUsed = false
		return
	}

	if inputStage, ok := asDoc(winningPlan["inputStage"]); ok {
		parseWinningPlan(inputStage, stats)
		return
	}
	if inputStages, ok := winningPlan["inputStages"].(bson.A); ok {
		for _, item := range inputStages {
			if sub, ok := asDoc(item); ok {
				parseWinningPlan(sub, stats)
				if stats.IndexUsed || stats.Stage == stageCollscan {
					return
				}
			}
		}
	}
}

func logStats(ctx context.Context, stats *Stats, collection string, stages, fields []string, logger *slog.Logger) {
	logLevel := slog.LevelInfo
	collscan := stats.Stage == stageCollscan
	var ratio float64
	lowSelectivity := false
	if stats.DocsExamined > 0 && stats.DocsReturned > 0 {
		ratio = float64(stats.DocsExamined) / float64(stats.DocsReturned)
		lowSelectivity = ratio > HighExaminationRatio
	}
	if collscan || lowSelectivity {
		logLevel = slog.LevelWarn
	}

	attrs := []any{
		"index_used", stats.IndexUsed,
		"stage", stats.Stage,
		"index_name", stats.IndexName,
		"docs_examined", stats.DocsExamined,
		"docs_returned", stats.DocsReturned,
		"execution_time_ms", stats.ExecutionTimeMillis,
		"keys_examined", stats.KeysExamined,
		"is_multi_key", stats.IsMultiKey,
	}
	if collection != "" {
		attrs = append(attrs, "collection", collection)
	}
	if len(stages) > 0 {
		attrs = append(attrs, "stages", stages)
	}
	if len(fields) > 0 {
		attrs = append(attrs, "fields", fields)
	}
	logger.Log(ctx, logLevel, "query execution plan analysis", attrs...)

	if collscan {
		logger.WarnContext(ctx, "performance alert: collection scan detected",
			"collection", collection,
			"fields", fields,
			"recommendation", "consider adding an index for the query filter fields",
			"impact", "high - query performance will degrade with collection size",
		)
	} else if lowSelectivity {
		logger.WarnContext(ctx, "performance alert: low query selectivity",
			"collection", collection,
			"docs_examined", stats.DocsExamined,
			"docs_returned", stats.DocsReturned,
			"selectivity_ratio", ratio,
			"recommendation", "consider optimizing query filters or index design",
		)
	}
}
