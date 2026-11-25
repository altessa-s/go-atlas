// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas-core/runtime/singleflight"
	"github.com/altessa-s/go-atlas/mapping/behavior"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	mongotranslator "github.com/altessa-s/go-atlas/mapping/behavior/translators/mongo"
	mongodocument "github.com/altessa-s/go-atlas/store/mongo/internal/document"
	converter "github.com/altessa-s/go-convert"
)

type repository struct {
	singleFlight   *singleflight.Group[string, any]
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

// deduplicate runs fn through the singleflight group under key, unless ctx
// carries a session: a read inside a transaction must see that transaction's
// snapshot, and a read outside it must never see its uncommitted writes, so
// session-bound reads are never shared in either direction.
func (r *repository) deduplicate(ctx context.Context, key string, fn func(context.Context) (any, error)) (any, error) {
	if mongo.SessionFromContext(ctx) != nil {
		return fn(ctx)
	}
	return r.singleFlight.Do(ctx, key, fn)
}

// deduplicationKey builds the singleflight key for a read, mixing in the
// optional per-context identity segment (WithDeduplicationIdentity) ahead of the
// db:collection base. When no identity func is configured, or it returns "", the
// key is identical to the un-prefixed form so existing callers are unaffected.
// The identity segment ensures two callers issuing the same filter concurrently
// are only collapsed when they share an identity — never across tenants.
func (r *repository) deduplicationKey(ctx context.Context, base string, filter bson.Raw) string {
	if r.dedupIdentity != nil {
		if id := r.dedupIdentity(ctx); id != "" {
			base = id + ":" + base
		}
	}
	return generateDeduplicationKey(base, filter)
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
// The function includes request deduplication using singleflight to prevent duplicate concurrent queries.
//
// This function is designed to be used with a Mongo instance for proper singleflight deduplication.
// The Mongo instance must be fully initialized with a connected singleFlight group.
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

	// Generate a deduplication key from filter for singleflight request deduplication.
	// Include database name to prevent cross-database data leakage, and the optional
	// per-context identity so identical filters from different callers never collapse.
	deduplicationKey := m.repo.deduplicationKey(ctx, col.Database().Name()+":"+col.Name(), raw)

	// Use singleflight to prevent duplicate concurrent requests
	data, err := m.repo.deduplicate(ctx, deduplicationKey, func(workCtx context.Context) (any, error) {
		// Apply timeout for database operation
		ctxWithTimeout, cancel := corecontext.WithMaxTimeout(workCtx, DefaultQueryTimeout)
		defer cancel()

		// Find the document in MongoDB
		var model T
		result := col.FindOne(ctxWithTimeout, raw)
		if err := result.Decode(&model); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return zero, mongo.ErrNoDocuments
			}
			return zero, coreerrs.WrapOperation(err, "decode document")
		}

		conv := sharedConverter[T](&m.repo)
		entity, convertErr := convertModel[T, E](conv, model)
		if convertErr != nil {
			return zero, coreerrs.WrapOperation(convertErr, "convert document")
		}
		return entity, nil
	})

	if err != nil {
		return zero, err
	}

	entity, ok := data.(E)
	if !ok {
		return zero, fmt.Errorf("singleflight type assertion failed: got %T, expected %T", data, zero)
	}

	return entity, nil
}

// GetEntities retrieves multiple entities from MongoDB by converting document models to domain entities.
// It uses generics to provide type-safe conversion from MongoDB document type T to domain entity type E.
// The function includes request deduplication using singleflight to prevent duplicate concurrent queries.
//
// This function is designed to be used with a Mongo instance for proper singleflight deduplication.
// The Mongo instance must be fully initialized with a connected singleFlight group.
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

	// Generate a deduplication key from filter for singleflight request deduplication.
	// Include database name to prevent cross-database data leakage, and the optional
	// per-context identity so identical filters from different callers never collapse.
	deduplicationKey := m.repo.deduplicationKey(ctx, col.Database().Name()+":"+col.Name()+deduplicationKeySuffixList, raw)

	// Use singleflight to prevent duplicate concurrent requests
	data, err := m.repo.deduplicate(ctx, deduplicationKey, func(workCtx context.Context) (any, error) {
		// Apply timeout for database operation
		ctxWithTimeout, cancel := corecontext.WithMaxTimeout(workCtx, DefaultQueryTimeout)
		defer cancel()

		// Execute a query to find matching documents
		cursor, err := col.Find(ctxWithTimeout, raw)
		if err != nil {
			return []E{}, coreerrs.WrapOperation(err, "execute query")
		}
		defer func() {
			closeCtx, closeCancel := context.WithTimeout(context.WithoutCancel(workCtx), DefaultCursorCloseTimeout)
			defer closeCancel()
			if cerr := cursor.Close(closeCtx); cerr != nil {
				// Log cursor close error but don't override main error
				// In production code, you might want to use a proper logger here
				_ = cerr // Explicitly ignore the error to satisfy linter
			}
		}()

		conv := sharedConverter[T](&m.repo)
		entities := make([]E, 0)
		for cursor.Next(ctxWithTimeout) {
			var model T
			if err = cursor.Decode(&model); err != nil {
				return nil, coreerrs.WrapOperation(err, "decode document")
			}
			entity, convertErr := convertModel[T, E](conv, model)
			if convertErr != nil {
				return nil, coreerrs.WrapOperation(convertErr, "convert document")
			}
			entities = append(entities, entity)
		}
		if err = cursor.Err(); err != nil {
			return nil, coreerrs.WrapOperation(err, "read documents")
		}

		return entities, nil
	})

	if err != nil {
		return nil, err
	}

	entities, ok := data.([]E)
	if !ok {
		return nil, fmt.Errorf("singleflight type assertion failed: got %T, expected []%T", data, *new(E))
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
