// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"fmt"
	"iter"
	"reflect"
	"slices"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/altessa-s/go-atlas-core/runtime/singleflight"
	"github.com/altessa-s/go-atlas/mapping/behavior"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	mongotranslator "github.com/altessa-s/go-atlas/mapping/behavior/translators/mongo"
	mongodocument "github.com/altessa-s/go-atlas/store/mongo/internal/document"
	converter "github.com/altessa-s/go-convert"
)

type repository struct {
	singleFlight   *singleflight.Group[readFlightKey, *readDocuments]
	dedupIdentity  DeduplicationIdentityFunc
	converterOpts  []converter.Option
	converters     sync.Map
	insertDocument *behavior.Engine[bson.M]
	updateDocument *behavior.Engine[bson.M]
	metrics        *mongoMetrics
}

func (r *repository) initTranslators(registry *bson.Registry, tag string, encryptedFields *mongodocument.Fields) {
	builderOptions := []mongotranslator.Option{
		mongotranslator.WithRegistry(registry),
		mongotranslator.WithBsonTagName(tag),
		mongotranslator.WithFieldTransformer(encryptedFields),
	}
	r.insertDocument = behavior.New(mongotranslator.NewInsertTranslator(builderOptions...))
	r.updateDocument = behavior.New(mongotranslator.NewUpdateTranslator(builderOptions...))
}

// ConvertToNewDocument builds an insert document using the client's registry and BSON layout.
// Encryption rules select fields by Go name in an [EncryptionModel] or by a struct tag
// such as encryption:"deterministic,email_key". Encryption failures return no document.
func (m *Mongo) ConvertToNewDocument(ctx context.Context, entity any) (bson.M, error) {
	return m.repo.insertDocument.Translate(ctx, entity)
}

// ConvertToUpdateDocument builds $set and $unset using the same field layout and encryption
// rules as [Mongo.ConvertToNewDocument]. Updates skip the root _id, identifiers and
// omitonupdate fields. A subdocument with protected descendants uses dotted update paths.
func (m *Mongo) ConvertToUpdateDocument(ctx context.Context, entity any) (bson.M, error) {
	return m.repo.updateDocument.Translate(ctx, entity)
}

type freshReadKey struct{}

// WithFreshRead bypasses concurrent read deduplication for [GetEntity] and
// [GetEntities]. Use it after a write to avoid joining a read that started
// before the write. The collection's read preference and concern still apply.
func WithFreshRead(ctx context.Context) context.Context {
	return context.WithValue(ctx, freshReadKey{}, true)
}

type readFlightKey struct {
	database, collection string
	handle               *mongo.Collection
	model                reflect.Type
	operation, identity  string
	filter               string
}

func (r *repository) deduplicationKey(ctx context.Context, col *mongo.Collection, model reflect.Type, operation string, filter bson.Raw) readFlightKey {
	key := readFlightKey{
		database: col.Database().Name(), collection: col.Name(), handle: col,
		model: model, operation: operation, filter: generateDeduplicationKey("", filter),
	}
	if r.dedupIdentity != nil {
		key.identity = r.dedupIdentity(ctx)
	}
	return key
}

func (r *repository) deduplicate(
	ctx context.Context, key readFlightKey, fresh bool, read func(context.Context) (*readDocuments, error),
) (*readDocuments, error) {
	// A session's reads must use its snapshot and cannot share uncommitted data.
	if fresh || ctx.Value(freshReadKey{}) != nil || mongo.SessionFromContext(ctx) != nil {
		return read(ctx)
	}
	return r.singleFlight.Do(ctx, key, read)
}

// readDocuments holds independent copies of the documents carried by the
// flight. The closed cursor retains the exact collection codec, including
// overrides on borrowed handles that the driver does not expose as settings.
type readDocuments struct {
	cursor *mongo.Cursor
	docs   []bson.Raw
	mu     sync.Mutex // serializes use of Cursor.Current and Decode
}

func (d *readDocuments) decode(raw bson.Raw, target any) error {
	owned := slices.Clone(raw)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cursor.Current = owned
	defer func() { d.cursor.Current = nil }()
	return d.cursor.Decode(target)
}

func readRepository[T any](
	ctx context.Context, r *repository, col *mongo.Collection, filter bson.Raw, single bool, settings []FilterEncoding,
) (*readDocuments, error) {
	operation := "find"
	if single {
		operation = "find_one"
	}
	key := r.deduplicationKey(ctx, col, reflect.TypeFor[T](), operation, filter)
	fresh := len(settings) > 0 && settings[len(settings)-1].FreshRead
	return r.deduplicate(ctx, key, fresh, func(workCtx context.Context) (*readDocuments, error) {
		queryCtx, cancel := corecontext.WithMaxTimeout(workCtx, DefaultQueryTimeout)
		defer cancel()

		opts := options.Find()
		if single {
			opts.SetLimit(-1) // As in FindOne, return at most one document in one batch.
		}
		cursor, err := col.Find(queryCtx, filter, opts)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "execute query")
		}
		defer func() {
			closeCtx, closeCancel := context.WithTimeout(context.WithoutCancel(workCtx), DefaultCursorCloseTimeout)
			defer closeCancel()
			_ = cursor.Close(closeCtx)
			cursor.Current = nil
		}()

		result := &readDocuments{cursor: cursor}
		for cursor.Next(queryCtx) {
			result.docs = append(result.docs, slices.Clone(cursor.Current))
			if single {
				break
			}
		}
		if err := cursor.Err(); err != nil {
			return nil, coreerrs.WrapOperation(err, "read documents")
		}
		if single && len(result.docs) == 0 {
			return nil, mongo.ErrNoDocuments
		}
		return result, nil
	})
}

func decodeEntity[T, E any](r *repository, docs *readDocuments, raw bson.Raw) (E, error) {
	var model T
	if err := docs.decode(raw, &model); err != nil {
		return *new(E), coreerrs.WrapOperation(err, "decode document")
	}
	entity, err := convertModel[T, E](sharedConverter[T](r), model)
	if err != nil {
		return *new(E), coreerrs.WrapOperation(err, "convert document")
	}
	return entity, nil
}

// sharedConverter returns the memoized converter for document model type T,
// building it on first use from the option set fixed at construction.
func sharedConverter[T any](r *repository) *converter.Converter[T, any] {
	t := reflect.TypeFor[T]()
	if v, ok := r.converters.Load(t); ok {
		return v.(*converter.Converter[T, any]) //nolint:errcheck // type guaranteed by Store
	}
	v, _ := r.converters.LoadOrStore(t, converter.New[T, any](r.converterOpts...))
	return v.(*converter.Converter[T, any]) //nolint:errcheck // type guaranteed by Store
}

// GetEntity retrieves a single entity from MongoDB by converting a document model to domain entity.
// It uses generics to provide type-safe conversion from MongoDB document type T to domain entity type E.
// Concurrent reads may share a query started before the caller's own write.
// Use [WithFreshRead] or [FilterEncoding.FreshRead] after a write to issue an independent query.
// Each read decodes and converts its own result.
//
// Type parameters:
//   - T: MongoDB document model type
//   - E: Domain entity type to convert to
//
// Parameters:
//   - ctx: Context for controlling query timeout and cancellation
//   - m: Mongo client instance containing singleflight for deduplication
//   - col: MongoDB collection to query
//   - filter: BSON filter criteria for finding the document
//
// Returns:
//   - E: The converted domain entity
//   - error: mongo.ErrNoDocuments if not found, or other errors for decode/conversion failures
//
// Example:
//
//	type UserModel struct { ID string `bson:"_id"` }
//	type UserEntity struct { ID string }
//
//	user, err := mongotools.GetEntity[UserModel, UserEntity](ctx, mongo, collection, bson.M{"_id": "123"})
func GetEntity[T any, E any](ctx context.Context, m *Mongo, col *mongo.Collection, filter any, settings ...FilterEncoding) (E, error) {
	var zero E

	raw, prepareErr := prepareFilter(ctx, col, filter, settings...)
	if prepareErr != nil {
		return zero, prepareErr
	}

	op := m.repo.metrics.operation("find_one", col.Name())
	op.total.Inc()
	stopOp := op.duration.Start()
	defer stopOp()

	docs, err := readRepository[T](ctx, &m.repo, col, raw, true, settings)
	if err != nil {
		return zero, err
	}
	return decodeEntity[T, E](&m.repo, docs, docs.docs[0])
}

// GetEntities retrieves multiple entities from MongoDB by converting document models to domain entities.
// It uses generics to provide type-safe conversion from MongoDB document type T to domain entity type E.
// Concurrent reads may share a query started before the caller's own write.
// Use [WithFreshRead] or [FilterEncoding.FreshRead] after a write to issue an independent query.
// Each read decodes and converts its own result.
//
// Type parameters:
//   - T: MongoDB document model type
//   - E: Domain entity type to convert to
//
// Parameters:
//   - ctx: Context for controlling query timeout and cancellation
//   - m: Mongo client instance containing singleflight for deduplication
//   - col: MongoDB collection to query
//   - filter: BSON filter criteria for finding documents
//
// Returns:
//   - []E: Slice of converted domain entities (nil if no documents found)
//   - error: Error for query execution, decode, or conversion failures
//
// Example:
//
//	type UserModel struct { ID string `bson:"_id"` }
//	type UserEntity struct { ID string }
//
//	users, err := mongotools.GetEntities[UserModel, UserEntity](ctx, mongo, collection, bson.M{"active": true})
func GetEntities[T any, E any](ctx context.Context, m *Mongo, col *mongo.Collection, filter any, settings ...FilterEncoding) ([]E, error) {
	raw, prepareErr := prepareFilter(ctx, col, filter, settings...)
	if prepareErr != nil {
		return nil, prepareErr
	}

	op := m.repo.metrics.operation("find", col.Name())
	op.total.Inc()
	stopOp := op.duration.Start()
	defer stopOp()

	docs, err := readRepository[T](ctx, &m.repo, col, raw, false, settings)
	if err != nil {
		return nil, err
	}
	entities := make([]E, 0, len(docs.docs))
	for _, raw := range docs.docs {
		entity, err := decodeEntity[T, E](&m.repo, docs, raw)
		if err != nil {
			return nil, err
		}
		entities = append(entities, entity)
	}
	return entities, nil
}

// GetEntitiesSeq streams matching entities without retaining the full result
// set. Each iteration yields either an entity or the first query, decode, or
// conversion error; after an error the sequence stops. Unlike GetEntities,
// this sequence is not deduplicated because each consumer controls its own
// iteration and cancellation.
func GetEntitiesSeq[T any, E any](ctx context.Context, m *Mongo, col *mongo.Collection, filter any, settings ...FilterEncoding) iter.Seq2[E, error] {
	return func(yield func(E, error) bool) {
		var zero E
		raw, err := prepareFilter(ctx, col, filter, settings...)
		if err != nil {
			yield(zero, err)
			return
		}
		op := m.repo.metrics.operation("find", col.Name())
		op.total.Inc()
		stopOp := op.duration.Start()
		defer stopOp()
		queryCtx, cancel := corecontext.WithMaxTimeout(ctx, DefaultQueryTimeout)
		defer cancel()
		cursor, err := col.Find(queryCtx, raw)
		if err != nil {
			yield(zero, coreerrs.WrapOperation(err, "execute query"))
			return
		}
		defer func() {
			closeCtx, closeCancel := context.WithTimeout(context.WithoutCancel(queryCtx), DefaultCursorCloseTimeout)
			defer closeCancel()
			_ = cursor.Close(closeCtx)
		}()

		conv := sharedConverter[T](&m.repo)
		for cursor.Next(queryCtx) {
			var model T
			if err = cursor.Decode(&model); err != nil {
				yield(zero, coreerrs.WrapOperation(err, "decode document"))
				return
			}
			entity, convertErr := convertModel[T, E](conv, model)
			if convertErr != nil {
				yield(zero, coreerrs.WrapOperation(convertErr, "convert document"))
				return
			}
			if !yield(entity, nil) {
				return
			}
		}
		if err = cursor.Err(); err != nil {
			yield(zero, coreerrs.WrapOperation(err, "read documents"))
		}
	}
}

func convertModel[T, E any](conv *converter.Converter[T, any], model T) (E, error) {
	var zero E
	dstType := reflect.TypeFor[E]()
	if dstType.Kind() == reflect.Pointer {
		dst := reflect.New(dstType.Elem()).Interface()
		if err := conv.ConvertE(model, dst); err != nil {
			return zero, err
		}
		entity, ok := dst.(E)
		if !ok {
			return zero, fmt.Errorf("mongo: converted entity has type %T, want %v", dst, dstType)
		}
		return entity, nil
	}
	dst := reflect.New(dstType).Interface()
	if err := conv.ConvertE(model, dst); err != nil {
		return zero, err
	}
	entity, ok := reflect.TypeAssert[E](reflect.ValueOf(dst).Elem())
	if !ok {
		return zero, fmt.Errorf("mongo: converted entity has type %T, want %v", dst, dstType)
	}
	return entity, nil
}
