// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongodb

import (
	"context"
	"errors"
	"iter"
	"maps"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/service/scheduler"

	mongotranslator "github.com/altessa-s/go-atlas/data/filter/translators/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	// DefaultTasksCollection is the MongoDB collection name used for persisting
	// [scheduler.TaskState] documents when no override is provided via [WithTasksCollection].
	DefaultTasksCollection = "scheduler_tasks"

	// DefaultHistoryCollection is the MongoDB collection name used for persisting
	// [scheduler.TaskHistory] documents when no override is provided via [WithHistoryCollection].
	DefaultHistoryCollection = "scheduler_history"
)

// Storage implements the [scheduler.Storage] interface using MongoDB as the
// backing store. Task states and execution history are persisted in separate
// collections within the same database.
//
// All methods are safe for concurrent use because they delegate to the
// underlying mongo-driver, which manages its own connection pool.
//
// Create a Storage with [New] and optionally call [Storage.EnsureIndexes]
// once at startup for optimal query performance.
type Storage struct {
	tasks   *mongo.Collection
	history *mongo.Collection
}

// New creates a [Storage] backed by the given MongoDB database. By default it
// uses [DefaultTasksCollection] and [DefaultHistoryCollection] as collection
// names; pass [WithTasksCollection] or [WithHistoryCollection] to override them.
//
// The returned Storage is ready to use immediately. Call [Storage.EnsureIndexes]
// once during application startup to create the indexes required for efficient
// queries.
//
// Example:
//
//	storage := mongodb.New(client.Database("mydb"))
//	s := scheduler.New(storage)
func New(db *mongo.Database, opts ...Option) *Storage {
	o := newOptions(opts...)

	return &Storage{
		tasks:   db.Collection(o.tasksCollection),
		history: db.Collection(o.historyCollection),
	}
}

// EnsureIndexes creates or updates MongoDB indexes on the tasks and history
// collections for optimal query performance. It should be called once during
// application startup. The call is idempotent: running it multiple times is
// safe and has no side effects beyond the initial index creation.
//
// The following indexes are created:
//   - tasks: compound (status, next_run_at), descending priority
//   - history: compound (task_id, started_at desc, _id desc), ascending ended_at
//
// Every key must match a bson tag in [taskDocument] / [historyDocument] — an
// index on a field name that no document carries is silently accepted by
// MongoDB and then never used.
//
// History retention is driven by [Storage.CleanupHistory], not by a TTL index:
// EndedAt is a Unix timestamp stored as int64, and MongoDB TTL indexes only
// expire documents whose indexed field holds a BSON date. The ascending
// ended_at index exists so that cleanup's range delete is served by an index.
//
// Returns an error if any index creation fails.
//
// Deployments created before this was corrected still carry the earlier
// indexes on the nonexistent fields next_run, start_time, and end_time. They
// are inert but consume write amplification; drop them manually.
func (s *Storage) EnsureIndexes(ctx context.Context) error {
	if _, err := s.tasks.Indexes().CreateMany(ctx, taskIndexModels()); err != nil {
		return err
	}

	_, err := s.history.Indexes().CreateMany(ctx, historyIndexModels())
	return err
}

// taskIndexModels returns the indexes maintained on the tasks collection.
// Every key must correspond to a bson tag on [taskDocument]; TestIndexKeys
// enforces that.
func taskIndexModels() []mongo.IndexModel {
	// No _id index is declared. MongoDB maintains a unique one automatically and
	// rejects any attempt to restate it — "the field 'unique' is not valid for
	// an _id index specification" — which made EnsureIndexes fail outright on
	// every call, taking down startup for anyone who checked its error.
	return []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "status", Value: 1}, {Key: "next_run_at", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "priority", Value: -1}},
		},
	}
}

// historyIndexModels returns the indexes maintained on the history collection.
// The compound key mirrors the sort used by both [Storage.History] and
// [Storage.HistoryPaginated] so each is served by this index; the ascending
// ended_at index serves [Storage.CleanupHistory]'s range delete.
func historyIndexModels() []mongo.IndexModel {
	return []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "task_id", Value: 1},
				{Key: "started_at", Value: -1},
				{Key: "_id", Value: -1},
			},
		},
		{
			Keys: bson.D{{Key: "ended_at", Value: 1}},
		},
	}
}

// GetTask retrieves the [scheduler.TaskState] for the given task ID.
// It returns (nil, nil) when no document matches the ID, allowing callers to
// distinguish "not found" from a storage error.
func (s *Storage) GetTask(ctx context.Context, id string) (*scheduler.TaskState, error) {
	var doc taskDocument
	err := s.tasks.FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil //nolint:nilnil // nil state with nil error indicates "not found"
		}
		return nil, err
	}

	return doc.toTaskState(), nil
}

// UpsertTask inserts a new [scheduler.TaskState] or replaces an existing one
// identified by state.ID. The entire document is replaced on update; partial
// field updates are not supported. The replacement runs as an update pipeline
// so the stored revision is incremented atomically with the write; $literal
// keeps "$"-prefixed string values from being read as field paths.
func (s *Storage) UpsertTask(ctx context.Context, state *scheduler.TaskState) error {
	doc := newTaskDocument(state)
	doc.Revision = 0
	update := mongo.Pipeline{bson.D{{Key: "$replaceWith", Value: bson.M{"$mergeObjects": bson.A{
		bson.M{"$literal": doc},
		bson.M{"revision": nextRevision},
	}}}}}

	_, err := s.tasks.UpdateOne(ctx, bson.M{"_id": doc.ID}, update, mongoOptions.UpdateOne().SetUpsert(true))
	return err
}

// CreateTask inserts the task document with revision one. The _id primary key
// makes the insert atomic insert-if-absent: a duplicate-key error means the
// task already exists and is reported as (false, nil).
func (s *Storage) CreateTask(ctx context.Context, state *scheduler.TaskState) (bool, error) {
	doc := newTaskDocument(state)
	doc.Revision = 1
	if _, err := s.tasks.InsertOne(ctx, doc); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// nextRevision is the aggregation expression for the stored revision plus one.
var nextRevision = bson.M{"$add": bson.A{bson.M{"$ifNull": bson.A{"$revision", 0}}, 1}}

// ReplaceTaskIf replaces the document via a single conditional ReplaceOne whose
// filter carries the fence, so the compare and the write are atomic. Fields
// stored with omitempty match their zero value when absent.
func (s *Storage) ReplaceTaskIf(ctx context.Context, state *scheduler.TaskState, expect scheduler.TaskFence) (bool, error) {
	filter := bson.M{
		"_id":            state.ID,
		"status":         int32(expect.Status),
		"next_run_at":    zeroOrMissing(expect.NextRunAt),
		"last_run_id":    zeroOrMissing(expect.LastRunID),
		"run_started_at": zeroOrMissing(expect.RunStartedAt),
		"revision":       zeroOrMissing(expect.Revision),
	}
	doc := newTaskDocument(state)
	doc.Revision = expect.Revision + 1
	res, err := s.tasks.ReplaceOne(ctx, filter, doc)
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}

// zeroOrMissing builds a filter value that also matches an absent field when v
// is the zero value, mirroring the omitempty encoding of the task document.
func zeroOrMissing[T comparable](v T) any {
	var zero T
	if v == zero {
		return bson.M{"$in": bson.A{zero, nil}}
	}
	return v
}

// ClaimRun applies the claim rule of [scheduler.Storage.ClaimRun] via a single
// conditional UpdateOne. The filter matches status==active, an absent or zero
// run_started_at (no unfinished run) and, when claim.NextRunAt is non-zero,
// next_run_at and run_at, so MongoDB's atomic document update guarantees that
// at most one concurrent caller flips the document and thus wins the claim.
func (s *Storage) ClaimRun(ctx context.Context, id string, claim scheduler.RunClaim) (bool, error) {
	if err := claim.Validate(); err != nil {
		return false, err
	}
	filter := bson.M{
		"_id":            id,
		"status":         int32(scheduler.TaskStatusActive),
		"run_started_at": zeroOrMissing(int64(0)),
	}
	if claim.NextRunAt != 0 {
		filter["next_run_at"] = claim.NextRunAt
		filter["run_at"] = zeroOrMissing(claim.RunAt)
	}
	update := bson.M{
		"$set": bson.M{
			"status":          int32(scheduler.TaskStatusRunning),
			"run_started_at":  claim.StartedAt,
			"last_run_id":     claim.RunID,
			"run_lease_until": claim.LeaseUntil,
			"run_lease_id":    claim.RunID,
			"updated_at":      claim.StartedAt,
		},
		"$inc": bson.M{"revision": 1},
	}
	res, err := s.tasks.UpdateOne(ctx, filter, update)
	if err != nil {
		return false, err
	}
	return res.ModifiedCount == 1, nil
}

// DeleteTask removes the task document and all associated history entries for
// the given task ID. Deleting a non-existent task is not considered an error.
// If the task deletion succeeds but history cleanup fails, the error from the
// history deletion is returned.
func (s *Storage) DeleteTask(ctx context.Context, id string) error {
	// Delete task state
	if _, err := s.tasks.DeleteOne(ctx, bson.M{"_id": id}); err != nil {
		return err
	}

	// Delete associated history
	_, err := s.history.DeleteMany(ctx, bson.M{"task_id": id})
	return err
}

// Tasks returns an iterator over all [scheduler.TaskState] documents, sorted by
// ID in ascending order. The underlying MongoDB cursor is opened lazily on first
// iteration and closed automatically when the iterator is exhausted or abandoned
// early via a break. If a cursor or decode error occurs, it is yielded as the
// error element and iteration stops.
func (s *Storage) Tasks(ctx context.Context) iter.Seq2[*scheduler.TaskState, error] {
	return func(yield func(*scheduler.TaskState, error) bool) {
		findOpts := mongoOptions.Find().SetSort(bson.D{{Key: "_id", Value: 1}})
		cursor, err := s.tasks.Find(ctx, bson.M{}, findOpts)
		if err != nil {
			yield(nil, err)
			return
		}
		defer func() { _ = cursor.Close(ctx) }()

		for cursor.Next(ctx) {
			var doc taskDocument
			if err := cursor.Decode(&doc); err != nil {
				yield(nil, err)
				return
			}
			if !yield(doc.toTaskState(), nil) {
				return
			}
		}

		if err := cursor.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// DueTasks returns an iterator over the task documents eligible for dispatch at
// now: status active and next_run_at at or before now, sorted by ID ascending.
// The predicate is served by the compound (status, next_run_at) index created
// in [Storage.EnsureIndexes], so a tick costs a range scan over the due tasks
// rather than a full collection fetch. The cursor lifecycle matches
// [Storage.Tasks].
func (s *Storage) DueTasks(ctx context.Context, now int64) iter.Seq2[*scheduler.TaskState, error] {
	return func(yield func(*scheduler.TaskState, error) bool) {
		query := bson.M{
			"status":      int32(scheduler.TaskStatusActive),
			"next_run_at": bson.M{"$lte": now},
		}
		findOpts := mongoOptions.Find().SetSort(bson.D{{Key: "_id", Value: 1}})
		cursor, err := s.tasks.Find(ctx, query, findOpts)
		if err != nil {
			yield(nil, err)
			return
		}
		defer func() { _ = cursor.Close(ctx) }()

		for cursor.Next(ctx) {
			var doc taskDocument
			if err := cursor.Decode(&doc); err != nil {
				yield(nil, err)
				return
			}
			if !yield(doc.toTaskState(), nil) {
				return
			}
		}

		if err := cursor.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// AddHistory inserts a [scheduler.TaskHistory] document into the history
// collection. Each call creates a new document; duplicates are not checked.
func (s *Storage) AddHistory(ctx context.Context, history *scheduler.TaskHistory) error {
	doc := newHistoryDocument(history)
	_, err := s.history.InsertOne(ctx, doc)
	return err
}

// History returns an iterator over [scheduler.TaskHistory] entries for the given
// task ID, ordered by StartedAt descending (most recent first) with _id
// descending as tie-breaker — the same total order [Storage.HistoryPaginated]
// produces. The cursor lifecycle follows the same semantics as [Storage.Tasks]:
// it is closed on exhaustion or early break, and errors are yielded inline.
func (s *Storage) History(ctx context.Context, id string) iter.Seq2[*scheduler.TaskHistory, error] {
	return func(yield func(*scheduler.TaskHistory, error) bool) {
		findOpts := mongoOptions.Find().SetSort(bson.D{
			{Key: "started_at", Value: -1},
			{Key: "_id", Value: -1},
		})
		cursor, err := s.history.Find(ctx, bson.M{"task_id": id}, findOpts)
		if err != nil {
			yield(nil, err)
			return
		}
		defer func() { _ = cursor.Close(ctx) }()

		for cursor.Next(ctx) {
			var doc historyDocument
			if err := cursor.Decode(&doc); err != nil {
				yield(nil, err)
				return
			}
			if !yield(doc.toTaskHistory(), nil) {
				return
			}
		}

		if err := cursor.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// CleanupHistory deletes all history entries whose EndedAt timestamp is older
// than the given retention duration measured from the current wall-clock time.
// For example, a retention of 7*24*time.Hour removes entries that ended more
// than seven days ago. Returns an error if the delete operation fails.
func (s *Storage) CleanupHistory(ctx context.Context, retention time.Duration) error {
	cutoff := time.Now().Add(-retention).Unix()
	_, err := s.history.DeleteMany(ctx, bson.M{"ended_at": bson.M{"$lt": cutoff}})
	return err
}

// TasksPaginated returns up to (pg.Limit+1) task states whose ID is
// lexicographically greater than pg.AfterID, sorted by ID ascending. When f
// is non-nil the filter AST is translated to a bson.M and merged into the
// query so MongoDB evaluates it server-side.
func (s *Storage) TasksPaginated(ctx context.Context, pg scheduler.Pagination, f filter.Node) ([]*scheduler.TaskState, error) {
	query := bson.M{}
	if pg.AfterID != "" {
		query["_id"] = bson.M{"$gt": pg.AfterID}
	}

	if f != nil {
		trans, err := mongotranslator.NewTranslator(
			filter.WithAllowedFields(scheduler.TaskFilterFields...),
			filter.WithFieldMapping(maps.Collect(taskFieldMapping.All())),
			filter.WithZeroWhenAbsent(maps.Collect(taskZeroFields.All())),
		)
		if err != nil {
			return nil, err
		}
		filterBson, err := trans.Translate(f)
		if err != nil {
			return nil, err
		}
		// Wrap existing query + filter in $and to avoid key conflicts
		query = bson.M{"$and": bson.A{query, filterBson}}
	}

	findOpts := mongoOptions.Find().
		SetSort(bson.D{{Key: "_id", Value: 1}}).
		SetLimit(pg.Limit + 1)

	cursor, err := s.tasks.Find(ctx, query, findOpts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	var results []*scheduler.TaskState
	for cursor.Next(ctx) {
		var doc taskDocument
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		results = append(results, doc.toTaskState())
	}
	return results, cursor.Err()
}

// HistoryPaginated returns up to (pg.Limit+1) history entries for taskID,
// sorted by StartedAt descending with ID descending as tie-breaker, starting
// after the cursor position in pg. When f is non-nil the filter AST is
// translated to a bson.M and merged into the query for server-side evaluation.
func (s *Storage) HistoryPaginated(ctx context.Context, taskID string, pg scheduler.HistoryPagination, f filter.Node) ([]*scheduler.TaskHistory, error) {
	query := bson.M{"task_id": taskID}
	if pg.AfterID != "" {
		// Compound cursor seek: entries with start_time < cursor, OR same
		// start_time but _id < cursor (descending order).
		query["$or"] = bson.A{
			bson.M{"started_at": bson.M{"$lt": pg.AfterStartedAt}},
			bson.M{"started_at": pg.AfterStartedAt, "_id": bson.M{"$lt": pg.AfterID}},
		}
	}

	if f != nil {
		trans, err := mongotranslator.NewTranslator(
			filter.WithAllowedFields(scheduler.HistoryFilterFields...),
			filter.WithFieldMapping(maps.Collect(historyFieldMapping.All())),
			filter.WithZeroWhenAbsent(maps.Collect(historyZeroFields.All())),
		)
		if err != nil {
			return nil, err
		}
		filterBson, err := trans.Translate(f)
		if err != nil {
			return nil, err
		}
		// Wrap existing query + filter in $and to avoid key conflicts
		query = bson.M{"$and": bson.A{query, filterBson}}
	}

	findOpts := mongoOptions.Find().
		SetSort(bson.D{{Key: "started_at", Value: -1}, {Key: "_id", Value: -1}}).
		SetLimit(pg.Limit + 1)

	cursor, err := s.history.Find(ctx, query, findOpts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	var results []*scheduler.TaskHistory
	for cursor.Next(ctx) {
		var doc historyDocument
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		results = append(results, doc.toTaskHistory())
	}
	return results, cursor.Err()
}

// Compile-time interface check
var _ scheduler.Storage = (*Storage)(nil)

// ownedRunFilter matches task id while runID owns its unfinished run, the
// ownership predicate of [scheduler.Storage.RenewRun] and
// [scheduler.Storage.FinishRun]: run_started_at present and non-zero.
func ownedRunFilter(id, runID string) bson.M {
	return bson.M{"_id": id, "last_run_id": runID, "run_started_at": bson.M{"$nin": bson.A{0, nil}}}
}

// FinishRun applies the finish transition of [scheduler.Storage.FinishRun] as
// a single conditional pipeline update, so the server evaluates ownership and
// the concurrent status and configuration together.
func (s *Storage) FinishRun(ctx context.Context, id, runID string, result scheduler.RunResult) (bool, error) {
	if runID == "" {
		return false, nil
	}
	oneShot := bson.M{"$ifNull": bson.A{"$one_shot", false}}
	// executed: a one-shot task still registered for the occurrence this run
	// executed (re-registration may have moved it to another RunAt meanwhile).
	executed := bson.M{"$and": bson.A{
		oneShot,
		bson.M{"$eq": bson.A{bson.M{"$ifNull": bson.A{"$run_at", 0}}, result.RunAt}},
	}}
	var failures any = 0
	if !result.Success {
		failures = bson.M{"$add": bson.A{bson.M{"$ifNull": bson.A{"$failures", 0}}, 1}}
	}
	update := mongo.Pipeline{bson.D{{Key: "$set", Value: bson.M{
		// A finished one-shot occurrence is terminal whatever the status; otherwise
		// a running task returns to active unless management changed its status.
		"status": bson.M{"$cond": bson.A{executed, scheduler.TaskStatusCompleted,
			bson.M{"$cond": bson.A{
				bson.M{"$eq": bson.A{"$status", scheduler.TaskStatusRunning}}, scheduler.TaskStatusActive, "$status",
			}},
		}},
		"next_run_at": bson.M{"$cond": bson.A{executed, 0,
			bson.M{"$cond": bson.A{oneShot, "$next_run_at",
				bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$schedule", bson.M{"$literal": result.Schedule}}}, result.NextRunAt, "$next_run_at"}},
			}},
		}},
		"last_run_at": result.StartedAt, "updated_at": result.EndedAt,
		"run_started_at": 0, "run_lease_until": 0, "run_lease_id": "", "failures": failures,
		"revision": nextRevision,
	}}}}
	res, err := s.tasks.UpdateOne(ctx, ownedRunFilter(id, runID), update)
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}

// RenewRun extends the lease of the unfinished run runID via a single
// conditional UpdateOne; see [scheduler.Storage.RenewRun].
func (s *Storage) RenewRun(ctx context.Context, id, runID string, leaseUntil int64) (bool, error) {
	if runID == "" {
		return false, nil
	}
	res, err := s.tasks.UpdateOne(ctx, ownedRunFilter(id, runID),
		bson.M{"$set": bson.M{"run_lease_until": leaseUntil, "run_lease_id": runID}, "$inc": bson.M{"revision": 1}},
	)
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}
