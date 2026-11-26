// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"bytes"
	"context"
	"errors"
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
	"github.com/altessa-s/go-atlas/store/mongo/internal/query"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	mongotranslator "github.com/altessa-s/go-atlas/mapping/behavior/translators/mongo"
	mongodocument "github.com/altessa-s/go-atlas/store/mongo/internal/document"
	converter "github.com/altessa-s/go-convert"
)

type repository struct {
	singleFlight   *singleflight.Group[readFlightKey, []bson.Raw]
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
	ctx context.Context, key readFlightKey, fresh bool, read func(context.Context) ([]bson.Raw, error),
) ([]bson.Raw, error) {
	// A session's reads must use its snapshot and cannot share uncommitted data.
	if fresh || ctx.Value(freshReadKey{}) != nil || mongo.SessionFromContext(ctx) != nil {
		return read(ctx)
	}
	return r.singleFlight.Do(ctx, key, read)
}

// readQuery visits a document while its bytes and driver decoder are valid.
func readQuery(ctx context.Context, col *mongo.Collection, filter bson.Raw, single bool, visit func(bson.Raw, func(any) error) error) error {
	queryCtx, cancel := corecontext.WithMaxTimeout(ctx, DefaultQueryTimeout)
	defer cancel()

	if single {
		// FindOne keeps the driver's server-side maxTimeMS calculation enabled.
		result := col.FindOne(queryCtx, filter)
		raw, err := result.Raw()
		if errors.Is(err, mongo.ErrNoDocuments) {
			return mongo.ErrNoDocuments
		}
		if err != nil {
			return coreerrs.WrapOperation(err, "execute query")
		}
		return visit(raw, result.Decode)
	}

	cursor, err := col.Find(queryCtx, filter)
	if err != nil {
		return coreerrs.WrapOperation(err, "execute query")
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.WithoutCancel(ctx), DefaultCursorCloseTimeout)
		defer closeCancel()
		_ = cursor.Close(closeCtx)
	}()

	for cursor.Next(queryCtx) {
		if err := visit(cursor.Current, cursor.Decode); err != nil {
			return err
		}
	}
	if err := cursor.Err(); err != nil {
		return coreerrs.WrapOperation(err, "read documents")
	}
	return nil
}

func readRepository[T, E any](
	ctx context.Context, m *Mongo, col *mongo.Collection, filter bson.Raw, single bool, settings []FilterEncoding,
) ([]E, error) {
	var encoding FilterEncoding
	if len(settings) > 0 {
		encoding = settings[len(settings)-1]
	}
	entities := []E{}
	decode := func(decodeModel func(any) error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var model T
		if err := decodeModel(&model); err != nil {
			return coreerrs.WrapOperation(err, "decode document")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		entity, err := convertModel[T, E](sharedConverter[T](&m.repo), model)
		if err != nil {
			return coreerrs.WrapOperation(err, "convert document")
		}
		entities = append(entities, entity)
		return ctx.Err()
	}

	owned := m.conn.externalClient == nil && col.Database().Client() == m.Client()
	if !owned && encoding.Registry == nil && encoding.BSONOptions == nil {
		// Preserve the driver's opaque BSON settings on borrowed handles.
		read := func() ([]E, error) {
			err := readQuery(ctx, col, filter, single, func(_ bson.Raw, driverDecode func(any) error) error {
				return decode(driverDecode)
			})
			if err != nil {
				return nil, err
			}
			return entities, nil
		}
		// A driver session must stay in the caller's goroutine.
		if mongo.SessionFromContext(ctx) != nil {
			return read()
		}
		return waitForRead(ctx, read)
	}

	operation := "find"
	if single {
		operation = "find_one"
	}
	key := m.repo.deduplicationKey(ctx, col, reflect.TypeFor[T](), operation, filter)
	docs, err := m.repo.deduplicate(ctx, key, encoding.FreshRead, func(workCtx context.Context) ([]bson.Raw, error) {
		var docs []bson.Raw
		err := readQuery(workCtx, col, filter, single, func(raw bson.Raw, _ func(any) error) error {
			docs = append(docs, slices.Clone(raw))
			return nil
		})
		return docs, err
	})
	if err != nil {
		return nil, err
	}

	reg, opts := query.FilterEncoding(col, encoding.Registry, encoding.BSONOptions)
	return waitForRead(ctx, func() ([]E, error) {
		entities = make([]E, 0, len(docs))
		for _, raw := range docs {
			if err := decode(func(target any) error {
				return decodeReadDocument(slices.Clone(raw), reg, opts, target)
			}); err != nil {
				return nil, err
			}
		}
		return entities, ctx.Err()
	})
}

// A BSON decoder or converter can run arbitrary code without taking a context.
// Allow callers to stop waiting and check cancellation before the next document.
func waitForRead[E any](ctx context.Context, read func() ([]E, error)) ([]E, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ctx.Done() == nil {
		return read()
	}
	type result struct {
		entities []E
		err      error
		panicVal any
	}
	done := make(chan result, 1)
	go func() {
		var r result
		defer func() {
			r.panicVal = recover()
			done <- r
		}()
		r.entities, r.err = read()
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-done:
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if r.panicVal != nil {
			panic(r.panicVal)
		}
		return r.entities, r.err
	}
}

func decodeReadDocument(raw bson.Raw, reg *bson.Registry, opts *options.BSONOptions, target any) error {
	dec := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(raw)))
	dec.SetRegistry(reg)
	if opts != nil {
		if opts.AllowTruncatingDoubles {
			dec.AllowTruncatingDoubles()
		}
		if opts.BinaryAsSlice {
			dec.BinaryAsSlice()
		}
		if opts.DefaultDocumentM {
			dec.DefaultDocumentM()
		}
		if opts.DefaultDocumentMap {
			dec.DefaultDocumentMap()
		}
		if opts.ObjectIDAsHexString {
			dec.ObjectIDAsHexString()
		}
		if opts.UseJSONStructTags {
			dec.UseJSONStructTags()
		}
		if opts.UseLocalTimeZone {
			dec.UseLocalTimeZone()
		}
		if opts.ZeroMaps {
			dec.ZeroMaps()
		}
		if opts.ZeroStructs {
			dec.ZeroStructs()
		}
	}
	return dec.Decode(target)
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
// Each read decodes and converts its own result. Reuse col across requests to
// deduplicate queries. For database or collection codec overrides, provide
// FilterEncoding. Borrowed handles use driver decoding and independent queries
// when their settings are unspecified.
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

	entities, err := readRepository[T, E](ctx, m, col, raw, true, settings)
	if err != nil {
		return zero, err
	}
	return entities[0], nil
}

// GetEntities retrieves multiple entities from MongoDB by converting document models to domain entities.
// It uses generics to provide type-safe conversion from MongoDB document type T to domain entity type E.
// Concurrent reads may share a query started before the caller's own write.
// Use [WithFreshRead] or [FilterEncoding.FreshRead] after a write to issue an independent query.
// Each read decodes and converts its own result. Reuse col across requests to
// deduplicate queries. For database or collection codec overrides, provide
// FilterEncoding. Borrowed handles use driver decoding and independent queries
// when their settings are unspecified.
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

	return readRepository[T, E](ctx, m, col, raw, false, settings)
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
