// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package explain

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	mongoopts "go.mongodb.org/mongo-driver/v2/mongo/options"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// White-box test covers the parser without requiring a database server.
func TestParseExplainResult(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		input    bson.M
		stage    string
		examined int64
	}{
		{"empty", bson.M{}, "UNKNOWN", 0},
		{"execution stats", bson.M{"executionStats": bson.M{"totalDocsExamined": int32(7), "winningPlan": bson.M{"stage": "COLLSCAN"}}}, "COLLSCAN", 7},
		{"planner", bson.M{"queryPlanner": bson.M{"winningPlan": bson.M{"stage": "IXSCAN", "indexName": "name_1"}}}, "IXSCAN", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseExplainResult(tc.input)
			require.Equal(t, tc.stage, got.Stage)
			require.Equal(t, tc.examined, got.DocsExamined)
			require.Equal(t, tc.stage == "IXSCAN", got.IndexUsed)
		})
	}
}

func TestParseExplainResult_SiblingExecutionStatsAndQueryPlanner(t *testing.T) {
	t.Parallel()

	t.Run("root siblings", func(t *testing.T) {
		t.Parallel()
		input := bson.M{
			"queryPlanner": bson.M{
				"winningPlan": bson.M{
					"stage": "FETCH",
					"inputStage": bson.M{
						"stage":      "IXSCAN",
						"indexName":  "email_1",
						"isMultiKey": true,
					},
				},
			},
			"executionStats": bson.M{
				"totalDocsExamined":   int32(5),
				"nReturned":           int32(5),
				"executionTimeMillis": int32(2),
				"totalKeysExamined":   int32(5),
			},
		}
		got := parseExplainResult(input)
		require.True(t, got.IndexUsed)
		require.Equal(t, "IXSCAN", got.Stage)
		require.Equal(t, "email_1", got.IndexName)
		require.Equal(t, int64(5), got.DocsExamined)
		require.Equal(t, int64(5), got.DocsReturned)
		require.Equal(t, int64(2), got.ExecutionTimeMillis)
		require.Equal(t, int64(5), got.KeysExamined)
		require.True(t, got.IsMultiKey)
	})

	t.Run("stages cursor siblings", func(t *testing.T) {
		t.Parallel()
		input := bson.M{
			"stages": bson.A{
				bson.M{
					"$cursor": bson.M{
						"queryPlanner": bson.M{
							"winningPlan": bson.M{
								"stage":     "IXSCAN",
								"indexName": "tenant_1",
							},
						},
						"executionStats": bson.M{
							"totalDocsExamined": int32(3),
							"nReturned":         int32(3),
						},
					},
				},
			},
		}
		got := parseExplainResult(input)
		require.True(t, got.IndexUsed)
		require.Equal(t, "IXSCAN", got.Stage)
		require.Equal(t, "tenant_1", got.IndexName)
		require.Equal(t, int64(3), got.DocsExamined)
		require.Equal(t, int64(3), got.DocsReturned)
	})
}

// This guard exercises logStats with the pipeline summary introduced by WP-11.
func TestLogStatsWithPipelineSummary_WarnsOnlyOnCollscan(t *testing.T) {
	t.Parallel()

	var buf lockedBuffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	logStats(t.Context(), &Stats{Stage: "UNKNOWN", IndexUsed: false}, "users", []string{"$match"}, []string{"email"}, logger)
	require.NotContains(t, buf.String(), "collection scan detected")
	require.NotContains(t, buf.String(), "level=WARN")

	buf.Reset()
	logStats(t.Context(), &Stats{Stage: "COLLSCAN", IndexUsed: false}, "users", []string{"$match"}, []string{"email"}, logger)
	require.Contains(t, buf.String(), "collection scan detected")
	require.Contains(t, buf.String(), "level=WARN")
}

func TestRun_RejectsUnknownVerbosityBeforeCommand(t *testing.T) {
	t.Parallel()

	deployment := drivertest.NewMockDeployment(bson.D{{Key: "ok", Value: 1}})
	opts := mongoopts.Client()
	opts.Deployment = deployment
	client, err := mongo.Connect(opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.WithoutCancel(t.Context())) })

	_, err = Run(t.Context(), client.Database("app").Collection("users"), bson.A{}, nil, nil,
		WithVerbosity(Verbosity("allPlansExecution")))
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
}

func TestRun_SendsOrderedExplainAndRedactsFilterMarkers(t *testing.T) {
	t.Parallel()

	const marker = "secret-filter-marker@example.com"
	const literalKey = "secret-literal-document-key"
	pipeline := bson.A{
		bson.D{{Key: "$match", Value: bson.D{
			{Key: "email", Value: marker},
			{Key: "$expr", Value: bson.D{{Key: "$literal", Value: bson.D{{Key: literalKey, Value: marker}}}}},
		}}},
	}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		deployment := drivertest.NewMockDeployment(bson.D{
			{Key: "ok", Value: 1},
			{Key: "queryPlanner", Value: bson.D{
				{Key: "parsedQuery", Value: bson.D{{Key: "email", Value: marker}}},
				{Key: "winningPlan", Value: bson.D{
					{Key: "stage", Value: "IXSCAN"},
					{Key: "indexName", Value: "email_1"},
				}},
			}},
		})
		opts := mongoopts.Client()
		opts.Deployment = deployment
		client, err := mongo.Connect(opts)
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Disconnect(context.WithoutCancel(t.Context())) })

		var buf lockedBuffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		stats, err := Run(t.Context(), client.Database("app").Collection("users"), pipeline, nil, logger)
		require.NoError(t, err)
		require.True(t, stats.IndexUsed)
		require.Equal(t, "email_1", stats.IndexName)
		require.NotContains(t, buf.String(), marker)
		require.NotContains(t, buf.String(), literalKey)
	})

	t.Run("failure", func(t *testing.T) {
		t.Parallel()
		deployment := drivertest.NewMockDeployment(bson.D{
			{Key: "ok", Value: 0},
			{Key: "code", Value: int32(2)},
			{Key: "codeName", Value: "BadValue"},
			{Key: "errmsg", Value: "invalid filter " + marker},
		})
		opts := mongoopts.Client()
		opts.Deployment = deployment
		client, err := mongo.Connect(opts)
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Disconnect(context.WithoutCancel(t.Context())) })

		var buf lockedBuffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		_, err = Run(t.Context(), client.Database("app").Collection("users"), pipeline, nil, logger)
		require.Error(t, err)
		require.NotContains(t, buf.String(), marker)
		require.NotContains(t, buf.String(), literalKey)
	})

	t.Run("raw output option logs raw explain", func(t *testing.T) {
		t.Parallel()
		deployment := drivertest.NewMockDeployment(bson.D{
			{Key: "ok", Value: 1},
			{Key: "queryPlanner", Value: bson.D{
				{Key: "parsedQuery", Value: bson.D{
					{Key: "email", Value: marker},
					{Key: "$literal", Value: bson.D{{Key: literalKey, Value: "secret-literal-value"}}},
				}},
				{Key: "winningPlan", Value: bson.D{
					{Key: "stage", Value: "IXSCAN"},
					{Key: "indexName", Value: "email_1"},
				}},
			}},
			{Key: "executionStats", Value: bson.D{
				{Key: "totalDocsExamined", Value: int32(2)},
				{Key: "nReturned", Value: int32(1)},
			}},
		})
		opts := mongoopts.Client()
		opts.Deployment = deployment
		client, err := mongo.Connect(opts)
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Disconnect(context.WithoutCancel(t.Context())) })

		var buf lockedBuffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		stats, err := Run(t.Context(), client.Database("app").Collection("users"), pipeline, nil, logger,
			WithVerbosity(VerbosityExecutionStats),
			WithInsecureLogRawExplain(),
		)
		require.NoError(t, err)
		require.Equal(t, int64(2), stats.DocsExamined)
		require.Equal(t, int64(1), stats.DocsReturned)
		require.Contains(t, buf.String(), marker)
		require.NotContains(t, buf.String(), literalKey)
		require.NotContains(t, buf.String(), "secret-literal-value")
	})
}
