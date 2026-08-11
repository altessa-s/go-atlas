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
	"strconv"
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

	// searchPageSize is how many documents each FT.SEARCH round-trip fetches.
	// Reads walk every matching document a page at a time via searchAll rather
	// than issuing one capped request.
	searchPageSize = 1000
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

// searchAll walks every document matching query, one page at a time, calling
// fn for each. Return false from fn to stop early.
//
// FT.SEARCH answers a single page per call and reports the full match count in
// Total. Issuing one request with a large LIMIT and consuming whatever comes
// back silently discards every match past that ceiling — for the task index
// that means tasks that simply stop being scheduled once the collection grows,
// with no error anywhere to explain it.
//
// Paging by offset is not a snapshot: documents written or deleted mid-walk can
// shift later pages, so a concurrent writer may cause an entry to be seen twice
// or missed. Every caller here either tolerates that (schedule reads converge on
// the next tick) or deletes what it collects afterwards rather than during.
func (s *Storage) searchAll(
	ctx context.Context,
	index, query string,
	sortBy []redis.FTSearchSortBy,
	noContent bool,
	fn func(redis.Document) (bool, error),
) error {
	for offset := 0; ; offset += searchPageSize {
		result, err := s.client.FTSearchWithArgs(ctx, index, query,
			&redis.FTSearchOptions{
				NoContent:   noContent,
				LimitOffset: offset,
				Limit:       searchPageSize,
				SortBy:      sortBy,
			},
		).Result()
		if err != nil {
			return err
		}

		for _, doc := range result.Docs {
			cont, fnErr := fn(doc)
			if fnErr != nil {
				return fnErr
			}
			if !cont {
				return nil
			}
		}

		if len(result.Docs) < searchPageSize || offset+len(result.Docs) >= result.Total {
			return nil
		}
	}
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

// claimRunScript atomically transitions a task JSON document from active→running
// for a specific occurrence. It runs entirely server-side under Redis's single-
// threaded execution, so the read-check-write is atomic: KEYS[1] is the task key;
// ARGV = [activeStatus, expectedNextRunAt, runningStatus, runStartedAt, jsonRunID].
// Returns 1 when this caller claimed the run, 0 otherwise.
const claimRunScript = `
local s = redis.call('JSON.GET', KEYS[1], '$.status')
if (not s) or s == '[]' then return 0 end
if tonumber(string.match(s, '(-?%d+)')) ~= tonumber(ARGV[1]) then return 0 end
local exp = tonumber(ARGV[2])
if exp ~= 0 then
  local nr = redis.call('JSON.GET', KEYS[1], '$.next_run_at')
  if (not nr) or nr == '[]' then return 0 end
  if tonumber(string.match(nr, '(-?%d+)')) ~= exp then return 0 end
end
redis.call('JSON.SET', KEYS[1], '$.status', ARGV[3])
redis.call('JSON.SET', KEYS[1], '$.run_started_at', ARGV[4])
redis.call('JSON.SET', KEYS[1], '$.updated_at', ARGV[4])
redis.call('JSON.SET', KEYS[1], '$.last_run_id', ARGV[5])
return 1
`

// ClaimRun atomically transitions the task from active→running for the occurrence
// scheduled at expectedNextRunAt by executing claimRunScript via EVAL. Redis runs
// the script atomically, so concurrent callers cannot both claim one occurrence.
func (s *Storage) ClaimRun(ctx context.Context, id string, expectedNextRunAt, runStartedAt int64, runID string) (bool, error) {
	res, err := s.client.Eval(ctx, claimRunScript, []string{s.taskKey(id)},
		int(scheduler.TaskStatusActive),
		expectedNextRunAt,
		int(scheduler.TaskStatusRunning),
		runStartedAt,
		strconv.Quote(runID), // JSON-encoded string for JSON.SET
	).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return false, coreerrs.WrapOperation(err, "claim task run")
	}
	n, _ := res.(int64)
	return n == 1, nil
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
		sortBy := []redis.FTSearchSortBy{{FieldName: "id", Asc: true}}
		err := s.searchAll(ctx, s.taskIndexName(), "*", sortBy, false,
			func(doc redis.Document) (bool, error) {
				td, parseErr := parseDocJSON[taskData](doc)
				if parseErr != nil {
					return false, coreerrs.WrapOperation(parseErr, "parse task document")
				}
				return yield(td.toTaskState(), nil), nil
			})
		if err != nil {
			yield(nil, coreerrs.WrapOperation(err, "search tasks"))
		}
	}
}

// DueTasks returns an iterator over the task states eligible for dispatch at
// now: status active and nextRunAt at or before now, sorted by ID ascending.
// Both fields are indexed as RediSearch numerics, so the predicate is evaluated
// server-side and a tick transfers only the due documents instead of the whole
// task index.
//
func (s *Storage) DueTasks(ctx context.Context, now int64) iter.Seq2[*scheduler.TaskState, error] {
	return func(yield func(*scheduler.TaskState, error) bool) {
		active := int(scheduler.TaskStatusActive)
		query := fmt.Sprintf("@status:[%d %d] @nextRunAt:[-inf %d]", active, active, now)

		sortBy := []redis.FTSearchSortBy{{FieldName: "id", Asc: true}}
		err := s.searchAll(ctx, s.taskIndexName(), query, sortBy, false,
			func(doc redis.Document) (bool, error) {
				td, parseErr := parseDocJSON[taskData](doc)
				if parseErr != nil {
					return false, coreerrs.WrapOperation(parseErr, "parse task document")
				}
				return yield(td.toTaskState(), nil), nil
			})
		if err != nil {
			yield(nil, coreerrs.WrapOperation(err, "search due tasks"))
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

// trimHistory removes the oldest history entries beyond the configured maximum.
//
// This runs on every AddHistory, so it is deliberately shaped to transfer only
// the overflow rather than the task's whole history. The first search asks for
// no documents at all — FT.SEARCH still reports the full match count — and the
// second fetches exactly the oldest (Total - max) keys, ascending. The earlier
// form pulled and sorted up to ten thousand documents on every single run of
// every single task just to discard all but the tail.
func (s *Storage) trimHistory(ctx context.Context, taskID string) {
	if ctx.Err() != nil {
		return
	}

	query := fmt.Sprintf("@taskId:{%s}", redisearch.EscapeTag(taskID))

	// Count only: LIMIT 0 0 returns Total without any document payload.
	counted, err := s.client.FTSearchWithArgs(ctx, s.historyIndexName(), query,
		&redis.FTSearchOptions{NoContent: true, LimitOffset: 0, Limit: 0},
	).Result()
	if err != nil {
		return
	}

	overflow := counted.Total - s.opts.maxHistoryPerTask
	if overflow <= 0 {
		return
	}

	oldest, err := s.client.FTSearchWithArgs(ctx, s.historyIndexName(), query,
		&redis.FTSearchOptions{
			NoContent:   true,
			LimitOffset: 0,
			Limit:       overflow,
			SortBy: []redis.FTSearchSortBy{
				{FieldName: "startedAt", Asc: true}, // oldest first
			},
		},
	).Result()
	if err != nil {
		return
	}

	pipe := s.client.Pipeline()
	for _, doc := range oldest.Docs {
		pipe.Del(ctx, doc.ID)
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

		sortBy := []redis.FTSearchSortBy{{FieldName: "startedAt", Asc: false}}
		err := s.searchAll(ctx, s.historyIndexName(), query, sortBy, false,
			func(doc redis.Document) (bool, error) {
				hd, parseErr := parseDocJSON[historyData](doc)
				if parseErr != nil {
					return false, coreerrs.WrapOperation(parseErr, "parse history document")
				}
				return yield(hd.toTaskHistory(), nil), nil
			})
		if err != nil && !errors.Is(err, redis.Nil) {
			yield(nil, coreerrs.WrapOperation(err, "search history"))
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

	// Collect first, delete after: deleting mid-walk would shift the offsets
	// the pagination is stepping through and leave expired entries behind.
	var keys []string
	if err := s.searchAll(ctx, s.historyIndexName(), query, nil, true,
		func(doc redis.Document) (bool, error) {
			keys = append(keys, doc.ID)
			return true, nil
		}); err != nil {
		return coreerrs.WrapOperation(err, "search expired history")
	}

	if len(keys) == 0 {
		return nil
	}

	pipe := s.client.Pipeline()
	for _, key := range keys {
		pipe.Del(ctx, key)
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

	var keys []string
	err := s.searchAll(ctx, s.historyIndexName(), query, nil, true,
		func(doc redis.Document) (bool, error) {
			keys = append(keys, doc.ID)
			return true, nil
		})
	if err != nil {
		// If index doesn't exist yet, fall back gracefully
		if strings.Contains(err.Error(), "no such index") {
			return nil, nil
		}
		return nil, err
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
		trans, err := redisearch.NewTranslator(
			maps.Collect(taskFieldSchema.All()),
			filter.WithAllowedFields(scheduler.TaskFilterFields...),
			filter.WithFieldMapping(maps.Collect(taskFieldMapping.All())),
		)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "build task filter translator")
		}
		translated, err := trans.Translate(f)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "translate task filter")
		}
		query = translated
	}

	var results []*scheduler.TaskState
	pastCursor := pg.AfterID == ""

	sortBy := []redis.FTSearchSortBy{{FieldName: "id", Asc: true}}
	err := s.searchAll(ctx, s.taskIndexName(), query, sortBy, false,
		func(doc redis.Document) (bool, error) {
			td, parseErr := parseDocJSON[taskData](doc)
			if parseErr != nil {
				return false, coreerrs.WrapOperation(parseErr, "parse task document")
			}
			state := td.toTaskState()
			if !pastCursor {
				if state.ID == pg.AfterID {
					pastCursor = true
				}
				return true, nil
			}
			results = append(results, state)
			return int64(len(results)) <= pg.Limit, nil
		})
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "search tasks paginated")
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
		trans, err := redisearch.NewTranslator(maps.Collect(historyFieldSchema.All()), filter.WithAllowedFields(scheduler.HistoryFilterFields...))
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "build history filter translator")
		}
		filterQuery, err := trans.Translate(f)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "translate history filter")
		}
		if filterQuery != "*" {
			query = "(" + query + " " + filterQuery + ")"
		}
	}

	var results []*scheduler.TaskHistory
	pastCursor := pg.AfterID == ""

	sortBy := []redis.FTSearchSortBy{{FieldName: "startedAt", Asc: false}}
	err := s.searchAll(ctx, s.historyIndexName(), query, sortBy, false,
		func(doc redis.Document) (bool, error) {
			hd, parseErr := parseDocJSON[historyData](doc)
			if parseErr != nil {
				return false, coreerrs.WrapOperation(parseErr, "parse history document")
			}
			entry := hd.toTaskHistory()
			if !pastCursor {
				if entry.StartedAt < pg.AfterStartedAt || (entry.StartedAt == pg.AfterStartedAt && entry.ID < pg.AfterID) {
					pastCursor = true
				}
				if !pastCursor {
					return true, nil
				}
			}
			results = append(results, entry)
			return int64(len(results)) <= pg.Limit, nil
		})
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "search history paginated")
	}

	return results, nil
}

// Compile-time interface check
var _ scheduler.Storage = (*Storage)(nil)
