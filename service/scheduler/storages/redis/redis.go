// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"maps"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/filter/translators/redisearch"
	"github.com/altessa-s/go-atlas/service/scheduler"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Key suffixes and constants
const (
	taskKeyPrefix    = "task"
	historyKeyPrefix = "history"
	idxKeyPrefix     = "idx"

	taskIndexSuffix    = "tasks"
	historyIndexSuffix = "history"

	// ftSearchLimit is the maximum number of results for FT.SEARCH queries.
	ftSearchLimit = 10000
)

// Storage provides a Redis-backed implementation of the [scheduler.Storage],
// [scheduler.Storage] interface
// using RedisJSON for document storage and RediSearch for indexed querying.
//
// All methods are safe for concurrent use by multiple goroutines.
// Thread safety is delegated to the underlying [redis.UniversalClient].
//
// Create a Storage with [New] and call [Storage.EnsureIndexes] once at startup.
//
// Key patterns used:
//   - Task state: {prefix}:task:{task_id} (JSON document via JSON.SET)
//   - History entry: {prefix}:history:{task_id}:{history_id} (JSON document via JSON.SET)
//   - Task index: {prefix}:idx:tasks (RediSearch FT index, created by [Storage.EnsureIndexes])
//   - History index: {prefix}:idx:history (RediSearch FT index, created by [Storage.EnsureIndexes])
type Storage struct {
	client    redis.UniversalClient
	keyPrefix string
	opts      *options
}

// New creates a new [Storage] backed by the given Redis client.
// It accepts zero or more [Option] values to override default configuration.
//
// New panics if client is nil. The client is used as-is; callers are
// responsible for its lifecycle (connection pooling, closing, etc.).
//
// After calling New, invoke [Storage.EnsureIndexes] to create the
// RediSearch indexes required for query operations.
//
// Example:
//
//	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	storage := redis.New(rdb, redis.WithKeyPrefix("myapp:scheduler"))
//	if err := storage.EnsureIndexes(ctx); err != nil { ... }
func New(client redis.UniversalClient, opts ...Option) *Storage {
	if client == nil {
		panic("redis client must not be nil")
	}

	o := newOptions(opts...)

	// Normalize prefix: remove trailing colon if present
	prefix := strings.TrimSuffix(o.keyPrefix, ":")

	return &Storage{
		client:    client,
		keyPrefix: prefix,
		opts:      o,
	}
}

// EnsureIndexes creates the RediSearch indexes required by [Storage] for
// task and history queries. It is idempotent: if the indexes already exist,
// no action is taken. Call this method once during application startup
// before invoking any query methods ([Storage.Tasks], [Storage.TasksPaginated],
// [Storage.History], [Storage.HistoryPaginated], or [Storage.CleanupHistory]).
//
// Returns an error if the index creation command fails for a reason other
// than the index already existing.
func (s *Storage) EnsureIndexes(ctx context.Context) error {
	if err := s.ensureTaskIndex(ctx); err != nil {
		return coreerrs.WrapOperation(err, "ensure task index")
	}
	if err := s.ensureHistoryIndex(ctx); err != nil {
		return coreerrs.WrapOperation(err, "ensure history index")
	}
	return nil
}

func (s *Storage) ensureTaskIndex(ctx context.Context) error {
	idxName := s.taskIndexName()

	// Check if index already exists
	_, err := s.client.FTInfo(ctx, idxName).Result()
	if err == nil {
		return nil // Index already exists
	}

	return s.client.FTCreate(ctx, idxName,
		&redis.FTCreateOptions{
			OnJSON:   true,
			Prefix:   []any{s.key(taskKeyPrefix, "")},
			NoFreqs:  true,
			NoFields: true,
		},
		&redis.FieldSchema{FieldName: "$.id", As: "id", FieldType: redis.SearchFieldTypeTag, Sortable: true},
		&redis.FieldSchema{FieldName: "$.description", As: "description", FieldType: redis.SearchFieldTypeText},
		&redis.FieldSchema{FieldName: "$.status", As: "status", FieldType: redis.SearchFieldTypeNumeric, Sortable: true},
		&redis.FieldSchema{FieldName: "$.priority", As: "priority", FieldType: redis.SearchFieldTypeNumeric, Sortable: true},
		&redis.FieldSchema{FieldName: "$.schedule", As: "schedule", FieldType: redis.SearchFieldTypeTag},
		&redis.FieldSchema{FieldName: "$.last_run_at", As: "lastRunAt", FieldType: redis.SearchFieldTypeNumeric, Sortable: true},
		&redis.FieldSchema{FieldName: "$.next_run_at", As: "nextRunAt", FieldType: redis.SearchFieldTypeNumeric, Sortable: true},
		&redis.FieldSchema{FieldName: "$.failures", As: "failures", FieldType: redis.SearchFieldTypeNumeric},
		&redis.FieldSchema{FieldName: "$.skip_next_run", As: "skipNextRun", FieldType: redis.SearchFieldTypeTag},
		&redis.FieldSchema{FieldName: "$.disable_history", As: "disableHistory", FieldType: redis.SearchFieldTypeTag},
		&redis.FieldSchema{FieldName: "$.unmanaged", As: "unmanaged", FieldType: redis.SearchFieldTypeTag},
		&redis.FieldSchema{FieldName: "$.one_shot", As: "oneShot", FieldType: redis.SearchFieldTypeTag},
		&redis.FieldSchema{FieldName: "$.created_at", As: "createdAt", FieldType: redis.SearchFieldTypeNumeric, Sortable: true},
		&redis.FieldSchema{FieldName: "$.updated_at", As: "updatedAt", FieldType: redis.SearchFieldTypeNumeric, Sortable: true},
	).Err()
}

func (s *Storage) ensureHistoryIndex(ctx context.Context) error {
	idxName := s.historyIndexName()

	// Check if index already exists
	_, err := s.client.FTInfo(ctx, idxName).Result()
	if err == nil {
		return nil // Index already exists
	}

	return s.client.FTCreate(ctx, idxName,
		&redis.FTCreateOptions{
			OnJSON:   true,
			Prefix:   []any{s.key(historyKeyPrefix, "")},
			NoFreqs:  true,
			NoFields: true,
		},
		&redis.FieldSchema{FieldName: "$.task_id", As: "taskId", FieldType: redis.SearchFieldTypeTag},
		&redis.FieldSchema{FieldName: "$.run_id", As: "runId", FieldType: redis.SearchFieldTypeTag},
		&redis.FieldSchema{FieldName: "$.started_at", As: "startedAt", FieldType: redis.SearchFieldTypeNumeric, Sortable: true},
		&redis.FieldSchema{FieldName: "$.ended_at", As: "endedAt", FieldType: redis.SearchFieldTypeNumeric, Sortable: true},
		&redis.FieldSchema{FieldName: "$.duration_ms", As: "durationMs", FieldType: redis.SearchFieldTypeNumeric},
		&redis.FieldSchema{FieldName: "$.success", As: "success", FieldType: redis.SearchFieldTypeTag},
		&redis.FieldSchema{FieldName: "$.error", As: "error", FieldType: redis.SearchFieldTypeText},
	).Err()
}

// key builds a Redis key with the configured prefix.
func (s *Storage) key(parts ...string) string {
	if s.keyPrefix == "" {
		return strings.Join(parts, ":")
	}
	return s.keyPrefix + ":" + strings.Join(parts, ":")
}

// taskKey returns the Redis key for a task.
func (s *Storage) taskKey(id string) string {
	return s.key(taskKeyPrefix, id)
}

// historyKey returns the Redis key for a history entry.
func (s *Storage) historyKey(taskID string, historyID string) string {
	return s.key(historyKeyPrefix, taskID, historyID)
}

// taskIndexName returns the RediSearch index name for tasks.
func (s *Storage) taskIndexName() string {
	return s.key(idxKeyPrefix, taskIndexSuffix)
}

// historyIndexName returns the RediSearch index name for history.
func (s *Storage) historyIndexName() string {
	return s.key(idxKeyPrefix, historyIndexSuffix)
}

// GetTask retrieves the state of a specific task by its unique identifier.
// If the task does not exist in Redis, it returns (nil, nil).
// Any Redis communication error is returned wrapped with the task ID.
func (s *Storage) GetTask(ctx context.Context, id string) (*scheduler.TaskState, error) {
	result, err := s.client.JSONGet(ctx, s.taskKey(id), "$").Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil //nolint:nilnil // nil state with nil error indicates "not found"
		}
		return nil, coreerrs.Wrapf(err, "failed to get task %q", id)
	}

	td, err := unmarshalJSONResult[taskData](result)
	if err != nil {
		return nil, coreerrs.Wrapf(err, "failed to unmarshal task %q", id)
	}

	return td.toTaskState(), nil
}

// UpsertTask creates or replaces the full task state document in Redis.
// The task is stored as a JSON document under the key {prefix}:task:{id}
// and is automatically indexed by RediSearch for query operations.
func (s *Storage) UpsertTask(ctx context.Context, state *scheduler.TaskState) error {
	td := newTaskData(state)

	if err := s.client.JSONSet(ctx, s.taskKey(state.ID), "$", td).Err(); err != nil {
		return coreerrs.Wrapf(err, "failed to save task %q", state.ID)
	}

	return nil
}

// DeleteTask removes a task state and all of its associated history entries
// from Redis. The deletion is performed in a single pipeline for efficiency,
// but is not transactional; a partial failure may leave orphaned history keys.
// If the task does not exist, DeleteTask succeeds silently.
func (s *Storage) DeleteTask(ctx context.Context, id string) error {
	// Find all history keys for this task via RediSearch
	historyKeys, err := s.findHistoryKeys(ctx, id)
	if err != nil {
		return coreerrs.Wrapf(err, "failed to find history for task %q", id)
	}

	pipe := s.client.Pipeline()

	// Delete task state
	pipe.Del(ctx, s.taskKey(id))

	// Delete all history entries
	for _, hKey := range historyKeys {
		pipe.Del(ctx, hKey)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return coreerrs.Wrapf(err, "failed to delete task %q", id)
	}

	return nil
}

// Tasks returns an iterator over all task states, sorted by ID in ascending
// order. The results are fetched in a single RediSearch FT.SEARCH query with
// an upper bound of 10 000 documents. If the iterator encounters a Redis or
// deserialization error, it yields (nil, err) and stops.
func (s *Storage) Tasks(ctx context.Context) iter.Seq2[*scheduler.TaskState, error] {
	return func(yield func(*scheduler.TaskState, error) bool) {
		result, err := s.client.FTSearchWithArgs(ctx, s.taskIndexName(), "*",
			&redis.FTSearchOptions{
				NoContent: false,
				Limit:     ftSearchLimit,
				SortBy: []redis.FTSearchSortBy{
					{FieldName: "id", Asc: true},
				},
			},
		).Result()
		if err != nil {
			yield(nil, coreerrs.WrapOperation(err, "search tasks"))
			return
		}

		for _, doc := range result.Docs {
			td, err := parseDocJSON[taskData](doc)
			if err != nil {
				yield(nil, coreerrs.WrapOperation(err, "parse task document"))
				return
			}
			if !yield(td.toTaskState(), nil) {
				return
			}
		}
	}
}

// AddHistory records a task execution history entry as a JSON document in Redis.
// If a history TTL is configured via [WithHistoryTTL], an expiration is set on
// the key. If a per-task maximum is configured via [WithMaxHistoryPerTask],
// entries exceeding the limit are trimmed on a best-effort basis (oldest first).
func (s *Storage) AddHistory(ctx context.Context, history *scheduler.TaskHistory) error {
	hd := newHistoryData(history)

	historyKey := s.historyKey(history.TaskID, history.ID)
	if err := s.client.JSONSet(ctx, historyKey, "$", hd).Err(); err != nil {
		return coreerrs.WrapOperation(err, "add history")
	}

	// Set TTL if configured
	if s.opts.historyTTL > 0 {
		s.client.Expire(ctx, historyKey, s.opts.historyTTL)
	}

	// Trim to max entries if configured
	if s.opts.maxHistoryPerTask > 0 {
		s.trimHistory(ctx, history.TaskID)
	}

	return nil
}

// trimHistory removes oldest history entries beyond the max limit.
func (s *Storage) trimHistory(ctx context.Context, taskID string) {
	if ctx.Err() != nil {
		return
	}

	escapedID := redisearch.EscapeTag(taskID)
	query := fmt.Sprintf("@taskId:{%s}", escapedID)

	result, err := s.client.FTSearchWithArgs(ctx, s.historyIndexName(), query,
		&redis.FTSearchOptions{
			NoContent: true,
			Limit:     ftSearchLimit,
			SortBy: []redis.FTSearchSortBy{
				{FieldName: "startedAt", Asc: false}, // most recent first
			},
		},
	).Result()
	if err != nil || result.Total <= s.opts.maxHistoryPerTask {
		return
	}

	// Delete entries beyond the limit
	pipe := s.client.Pipeline()
	for i := s.opts.maxHistoryPerTask; i < result.Total && i < len(result.Docs); i++ {
		pipe.Del(ctx, result.Docs[i].ID)
	}
	_, _ = pipe.Exec(ctx) //nolint:errcheck // best-effort trim
}

// History returns an iterator over execution history entries for the given task,
// ordered by start time descending (most recent first). If the task has no
// history or does not exist, the iterator yields zero elements. A redis.Nil
// error from the search is treated as empty (not propagated).
func (s *Storage) History(ctx context.Context, id string) iter.Seq2[*scheduler.TaskHistory, error] {
	return func(yield func(*scheduler.TaskHistory, error) bool) {
		escapedID := redisearch.EscapeTag(id)
		query := fmt.Sprintf("@taskId:{%s}", escapedID)

		result, err := s.client.FTSearchWithArgs(ctx, s.historyIndexName(), query,
			&redis.FTSearchOptions{
				NoContent: false,
				Limit:     ftSearchLimit,
				SortBy: []redis.FTSearchSortBy{
					{FieldName: "startedAt", Asc: false},
				},
			},
		).Result()
		if err != nil {
			if !errors.Is(err, redis.Nil) {
				yield(nil, coreerrs.WrapOperation(err, "search history"))
			}
			return
		}

		for _, doc := range result.Docs {
			hd, err := parseDocJSON[historyData](doc)
			if err != nil {
				yield(nil, coreerrs.WrapOperation(err, "parse history document"))
				return
			}
			if !yield(hd.toTaskHistory(), nil) {
				return
			}
		}
	}
}

// CleanupHistory deletes all history entries whose endedAt timestamp is older
// than the specified retention duration relative to the current time. Entries
// are discovered via a RediSearch range query and deleted in a single pipeline.
// Returns nil if no expired entries are found.
func (s *Storage) CleanupHistory(ctx context.Context, retention time.Duration) error {
	cutoff := time.Now().Add(-retention).Unix()

	query := fmt.Sprintf("@endedAt:[-inf %d]", cutoff)
	result, err := s.client.FTSearchWithArgs(ctx, s.historyIndexName(), query,
		&redis.FTSearchOptions{
			NoContent: true,
			Limit:     ftSearchLimit,
		},
	).Result()
	if err != nil {
		return coreerrs.WrapOperation(err, "search expired history")
	}

	if len(result.Docs) == 0 {
		return nil
	}

	pipe := s.client.Pipeline()
	for _, doc := range result.Docs {
		pipe.Del(ctx, doc.ID)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return coreerrs.WrapOperation(err, "delete expired history")
	}

	return nil
}

// findHistoryKeys returns all Redis keys for a task's history entries via RediSearch.
func (s *Storage) findHistoryKeys(ctx context.Context, taskID string) ([]string, error) {
	escapedID := redisearch.EscapeTag(taskID)
	query := fmt.Sprintf("@taskId:{%s}", escapedID)

	result, err := s.client.FTSearchWithArgs(ctx, s.historyIndexName(), query,
		&redis.FTSearchOptions{
			NoContent: true,
			Limit:     ftSearchLimit,
		},
	).Result()
	if err != nil {
		// If index doesn't exist yet, fall back gracefully
		if strings.Contains(err.Error(), "no such index") {
			return nil, nil
		}
		return nil, err
	}

	keys := make([]string, 0, len(result.Docs))
	for _, doc := range result.Docs {
		keys = append(keys, doc.ID)
	}
	return keys, nil
}

// unmarshalJSONResult parses the JSON array wrapper returned by JSONGet("$").
// RedisJSON returns `[{...}]` for root path queries.
func unmarshalJSONResult[T any](raw string) (*T, error) {
	var arr []T
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return nil, err
	}
	if len(arr) == 0 {
		return nil, errors.New("empty JSON result")
	}
	return &arr[0], nil
}

// parseDocJSON extracts and parses the JSON body from an FT.SEARCH document.
func parseDocJSON[T any](doc redis.Document) (*T, error) {
	jsonStr, ok := doc.Fields["$"]
	if !ok {
		return nil, errors.New("document missing '$' field")
	}
	var v T
	if err := json.Unmarshal([]byte(jsonStr), &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// TasksPaginated returns up to (pg.Limit+1) task states whose ID is
// lexicographically greater than pg.AfterID, sorted by ID ascending. When f
// is non-nil the filter AST is translated to a RediSearch query for server-side
// evaluation. RediSearch does not support cursor-seek on TAG fields, so
// client-side skip past AfterID is still applied.
func (s *Storage) TasksPaginated(ctx context.Context, pg scheduler.Pagination, f filter.Node) ([]*scheduler.TaskState, error) {
	query := "*"
	if f != nil {
		trans := redisearch.NewTranslator(maps.Collect(taskFieldSchema.All()), filter.WithAllowedFields(scheduler.TaskFilterFields...), filter.WithFieldMapping(maps.Collect(taskFieldMapping.All())))
		translated, err := trans.Translate(f)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "translate task filter")
		}
		query = translated
	}

	result, err := s.client.FTSearchWithArgs(ctx, s.taskIndexName(), query,
		&redis.FTSearchOptions{
			NoContent: false,
			Limit:     ftSearchLimit,
			SortBy: []redis.FTSearchSortBy{
				{FieldName: "id", Asc: true},
			},
		},
	).Result()
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "search tasks paginated")
	}

	var results []*scheduler.TaskState
	pastCursor := pg.AfterID == ""

	for _, doc := range result.Docs {
		td, err := parseDocJSON[taskData](doc)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "parse task document")
		}
		state := td.toTaskState()
		if !pastCursor {
			if state.ID == pg.AfterID {
				pastCursor = true
			}
			continue
		}
		results = append(results, state)
		if int64(len(results)) > pg.Limit {
			break
		}
	}

	return results, nil
}

// HistoryPaginated returns up to (pg.Limit+1) history entries for taskID,
// sorted by StartedAt descending with ID descending as tie-breaker, starting
// after the cursor position in pg. When f is non-nil the filter AST is
// translated to a RediSearch query and combined with the taskId constraint.
// Uses RediSearch with client-side cursor skip.
func (s *Storage) HistoryPaginated(ctx context.Context, taskID string, pg scheduler.HistoryPagination, f filter.Node) ([]*scheduler.TaskHistory, error) {
	escapedID := redisearch.EscapeTag(taskID)
	query := fmt.Sprintf("@taskId:{%s}", escapedID)

	if f != nil {
		trans := redisearch.NewTranslator(maps.Collect(historyFieldSchema.All()), filter.WithAllowedFields(scheduler.HistoryFilterFields...))
		filterQuery, err := trans.Translate(f)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "translate history filter")
		}
		if filterQuery != "*" {
			query = "(" + query + " " + filterQuery + ")"
		}
	}

	result, err := s.client.FTSearchWithArgs(ctx, s.historyIndexName(), query,
		&redis.FTSearchOptions{
			NoContent: false,
			Limit:     ftSearchLimit,
			SortBy: []redis.FTSearchSortBy{
				{FieldName: "startedAt", Asc: false},
			},
		},
	).Result()
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "search history paginated")
	}

	var results []*scheduler.TaskHistory
	pastCursor := pg.AfterID == ""

	for _, doc := range result.Docs {
		hd, err := parseDocJSON[historyData](doc)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "parse history document")
		}
		entry := hd.toTaskHistory()
		if !pastCursor {
			if entry.StartedAt < pg.AfterStartedAt || (entry.StartedAt == pg.AfterStartedAt && entry.ID < pg.AfterID) {
				pastCursor = true
			}
			if !pastCursor {
				continue
			}
		}
		results = append(results, entry)
		if int64(len(results)) > pg.Limit {
			break
		}
	}

	return results, nil
}

// Compile-time interface check
var _ scheduler.Storage = (*Storage)(nil)
