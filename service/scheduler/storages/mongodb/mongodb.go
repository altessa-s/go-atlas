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
//   - tasks: unique on _id, compound (status, next_run), descending priority
//   - history: compound (task_id, start_time desc), TTL on end_time
//
// Returns an error if any index creation fails.
func (s *Storage) EnsureIndexes(ctx context.Context) error {
	// Tasks collection indexes
	taskIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "_id", Value: 1}},
			Options: mongoOptions.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "status", Value: 1}, {Key: "next_run", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "priority", Value: -1}},
		},
	}

	if _, err := s.tasks.Indexes().CreateMany(ctx, taskIndexes); err != nil {
		return err
	}

	// History collection indexes
	historyIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "task_id", Value: 1}, {Key: "start_time", Value: -1}},
		},
		{
			Keys:    bson.D{{Key: "end_time", Value: 1}},
			Options: mongoOptions.Index().SetExpireAfterSeconds(0), // TTL index placeholder
		},
	}

	_, err := s.history.Indexes().CreateMany(ctx, historyIndexes)
	return err
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
// field updates are not supported.
func (s *Storage) UpsertTask(ctx context.Context, state *scheduler.TaskState) error {
	doc := newTaskDocument(state)
	replaceOpts := mongoOptions.Replace().SetUpsert(true)

	_, err := s.tasks.ReplaceOne(ctx, bson.M{"_id": doc.ID}, doc, replaceOpts)
	return err
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

// AddHistory inserts a [scheduler.TaskHistory] document into the history
// collection. Each call creates a new document; duplicates are not checked.
func (s *Storage) AddHistory(ctx context.Context, history *scheduler.TaskHistory) error {
	doc := newHistoryDocument(history)
	_, err := s.history.InsertOne(ctx, doc)
	return err
}

// History returns an iterator over [scheduler.TaskHistory] entries for the given
// task ID, ordered by start time descending (most recent first). The cursor
// lifecycle follows the same semantics as [Storage.Tasks]: it is closed on
// exhaustion or early break, and errors are yielded inline.
func (s *Storage) History(ctx context.Context, id string) iter.Seq2[*scheduler.TaskHistory, error] {
	return func(yield func(*scheduler.TaskHistory, error) bool) {
		findOpts := mongoOptions.Find().SetSort(bson.D{{Key: "start_time", Value: -1}})
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
