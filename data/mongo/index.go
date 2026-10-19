// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// MongoDB error codes of an index that conflicts with an existing one.
const (
	// MongoErrorCodeIndexOptionsConflict: an index with the name exists with other options, or the keys exist under another name.
	MongoErrorCodeIndexOptionsConflict = 85
	// MongoErrorCodeIndexKeySpecsConflict: an index with the name exists with other keys.
	MongoErrorCodeIndexKeySpecsConflict = 86
)

// idIndexName is the name of the implicit _id index, which can never be dropped.
const idIndexName = "_id_"

// ErrInvalidIndexModels is returned by [EnsureIndexes] for models whose names collide, an obsolete name that is also a model's or
// empty, an attempt to drop the _id index, or keys a name cannot be derived from.
var ErrInvalidIndexModels = errors.New("mongo: invalid index models")

// EnsureIndexes makes the indexes of coll match models; it is idempotent and safe to run at every start. Without options it is one
// createIndexes command: an index that already exists with the same specification is a no-op, and one that conflicts (same name with
// other keys or options, or the same keys under another name) fails it with the server's error.
//
// [WithDropObsolete] first drops the named indexes when present (a missing collection or index is fine). [WithDropOnSpecMismatch]
// resolves a conflict by name: when creating a model fails with IndexOptionsConflict or IndexKeySpecsConflict and an index with the
// model's name exists, that index is dropped and the model created once more; a conflict with an index of another name is returned,
// never resolved by dropping it. The server decides what a mismatch is, so an equal index is never rebuilt. Dropping a unique index
// opens a window without the constraint until the new one is built, and a rebuild fails if the data violates the new specification.
//
// A model without a name gets the name the driver would give it (keys joined as "field_1_other_-1"). Models must have distinct names,
// and an obsolete name may not be a model's, empty, or "_id_" ([ErrInvalidIndexModels]).
func EnsureIndexes(ctx context.Context, coll *mongo.Collection, models []mongo.IndexModel, opts ...IndexOption) error {
	o := newIndexOptions(opts...)
	names, err := indexModelNames(models)
	if err != nil {
		return err
	}
	for _, name := range o.dropObsolete {
		switch {
		case name == "" || name == idIndexName:
			return fmt.Errorf("%w: cannot drop index %q", ErrInvalidIndexModels, name)
		case slices.Contains(names, name):
			return fmt.Errorf("%w: index %q is both a model and obsolete", ErrInvalidIndexModels, name)
		}
	}

	for _, name := range o.dropObsolete {
		if dropErr := dropIndexIfExists(ctx, coll, name); dropErr != nil {
			return dropErr
		}
	}
	if len(models) == 0 {
		return nil
	}

	models = namedModels(models, names)
	_, err = coll.Indexes().CreateMany(ctx, models)
	switch {
	case err == nil:
		return nil
	case !o.dropOnSpecMismatch || !isIndexConflict(err):
		return coreerrs.WrapOperationWithContext(err, "create indexes", coll.Name())
	}
	return recreateMismatched(ctx, coll, models, names)
}

// Index reconciliation lock: [WithDropOnSpecMismatch] drops and recreates indexes only while holding a per-collection mutex
// document in [MigrationLockCollectionName], so replicas starting together cannot drop each other's freshly built index.
const (
	// IndexLockLease is the lease of the reconciliation lock; it bounds how long a crashed holder blocks the others and must
	// cover the index builds of one reconciliation.
	IndexLockLease = 2 * time.Minute
	// IndexLockWaitTimeout bounds how long EnsureIndexes waits for a peer's reconciliation of the same collection.
	IndexLockWaitTimeout = IndexLockLease + 30*time.Second

	indexLockPrefix = "indexes:"
)

// recreateMismatched reconciles the models under the collection's reconciliation lock: each model is created afresh, and only
// when that fails with a conflict while an index of the model's name exists whose current specification differs from the model is
// that index dropped and the model created again. A peer that already replaced the index makes the fresh create succeed, so a
// correct index is never dropped on the strength of a stale conflict.
func recreateMismatched(ctx context.Context, coll *mongo.Collection, models []mongo.IndexModel, names []string) error {
	locks := coll.Database().Collection(MigrationLockCollectionName)
	id := indexLockPrefix + coll.Name()
	owner := bson.NewObjectID()

	acquireCtx, cancel := corecontext.WithMaxTimeout(ctx, IndexLockWaitTimeout)
	defer cancel()
	if err := acquireDatabaseLock(acquireCtx, locks, id, owner, IndexLockLease); err != nil {
		return err
	}
	defer func() { _ = releaseDatabaseLock(ctx, locks, id, owner) }() //nolint:errcheck // best effort: the lease bounds a failed release

	workCtx, workCancel := corecontext.WithMaxTimeout(ctx, IndexLockLease)
	defer workCancel()
	for i, model := range models {
		if err := reconcileIndex(workCtx, coll, model, names[i]); err != nil {
			return err
		}
	}
	return nil
}

// reconcileIndex creates model (named name), replacing an existing index of that name whose specification differs.
func reconcileIndex(ctx context.Context, coll *mongo.Collection, model mongo.IndexModel, name string) error {
	_, err := coll.Indexes().CreateOne(ctx, model)
	if err == nil {
		return nil
	}
	if !isIndexConflict(err) {
		return coreerrs.WrapOperationWithContext(err, "create index", coll.Name()+"."+name)
	}
	current, found, listErr := indexSpecification(ctx, coll, name)
	if listErr != nil {
		return listErr
	}
	if !found || indexMatchesModel(current, model) {
		// The conflict is with an index of another name (same keys): not ours to drop.
		return coreerrs.WrapOperationWithContext(err, "create index", coll.Name()+"."+name)
	}
	if err := dropIndexIfExists(ctx, coll, name); err != nil {
		return err
	}
	if _, err := coll.Indexes().CreateOne(ctx, model); err != nil {
		return coreerrs.WrapOperationWithContext(err, "recreate mismatched index", coll.Name()+"."+name)
	}
	return nil
}

// indexSpecification returns the full listIndexes document of index name of coll.
func indexSpecification(ctx context.Context, coll *mongo.Collection, name string) (bson.Raw, bool, error) {
	cur, err := coll.Indexes().List(ctx)
	if err != nil {
		if IsErrorCollectionNotFound(err) {
			return nil, false, nil
		}
		return nil, false, coreerrs.WrapOperationWithContext(err, "list indexes", coll.Name())
	}
	var specs []bson.Raw
	if err := cur.All(ctx, &specs); err != nil {
		return nil, false, coreerrs.WrapOperationWithContext(err, "list indexes", coll.Name())
	}
	for _, spec := range specs {
		if n, ok := spec.Lookup("name").StringValueOK(); ok && n == name {
			return spec, true, nil
		}
	}
	return nil, false, nil
}

// indexComparedOptions are the listIndexes fields indexMatchesModel compares; any other option field on either side makes the
// two not provably equal.
var indexComparedOptions = coremaps.NewImmutableMap(map[string]struct{}{
	"v": {}, "key": {}, "name": {}, "ns": {}, "unique": {}, "sparse": {}, "expireAfterSeconds": {}, "partialFilterExpression": {},
	"hidden": {},
})

// indexMatchesModel reports whether the stored specification spec provably equals model: same keys (numerically equal values in
// the same order) and the same unique, sparse, hidden, expireAfterSeconds and partialFilterExpression. It is conservative: an
// option it does not compare (collation, text or geo settings, ...) on either side yields false. It only vetoes drops: an index
// it reports equal is never dropped.
func indexMatchesModel(spec bson.Raw, model mongo.IndexModel) bool {
	args, err := resolveIndexOptions(model)
	if err != nil {
		return false
	}
	if args.Collation != nil || args.Weights != nil || args.DefaultLanguage != nil || args.LanguageOverride != nil ||
		args.TextVersion != nil || args.SphereVersion != nil || args.Bits != nil || args.Max != nil || args.Min != nil ||
		args.StorageEngine != nil || args.WildcardProjection != nil {
		return false
	}
	elems, err := spec.Elements()
	if err != nil {
		return false
	}
	for _, e := range elems {
		if !indexComparedOptions.Contains(e.Key()) {
			return false
		}
	}
	keys, err := bson.Marshal(model.Keys)
	if err != nil || !sameKeys(spec.Lookup("key"), keys) {
		return false
	}
	boolOpt := func(field string, want *bool) bool {
		got, _ := spec.Lookup(field).BooleanOK()
		return got == (want != nil && *want)
	}
	if !boolOpt("unique", args.Unique) || !boolOpt("sparse", args.Sparse) || !boolOpt("hidden", args.Hidden) {
		return false
	}
	ttl, hasTTL := spec.Lookup("expireAfterSeconds").AsInt64OK()
	if hasTTL != (args.ExpireAfterSeconds != nil) || (hasTTL && ttl != int64(*args.ExpireAfterSeconds)) {
		return false
	}
	partial := spec.Lookup("partialFilterExpression")
	if args.PartialFilterExpression == nil {
		return partial.Type == 0
	}
	want, err := bson.Marshal(args.PartialFilterExpression)
	return err == nil && partial.Type == bson.TypeEmbeddedDocument && bytes.Equal(partial.Value, want)
}

// resolveIndexOptions applies the option setters of model (none for nil options) and returns the resulting index options.
func resolveIndexOptions(model mongo.IndexModel) (mongoOptions.IndexOptions, error) {
	var args mongoOptions.IndexOptions
	if model.Options == nil {
		return args, nil
	}
	for _, set := range model.Options.List() {
		if err := set(&args); err != nil {
			return mongoOptions.IndexOptions{}, err
		}
	}
	return args, nil
}

// sameKeys reports whether the stored key document equals the marshaled model keys: the same fields in the same order with
// numerically equal or identical string values.
func sameKeys(stored bson.RawValue, model []byte) bool {
	doc, ok := stored.DocumentOK()
	if !ok {
		return false
	}
	got, err := doc.Elements()
	if err != nil {
		return false
	}
	want, err := bson.Raw(model).Elements()
	if err != nil || len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i].Key() != want[i].Key() {
			return false
		}
		g, w := got[i].Value(), want[i].Value()
		if gs, ok := g.StringValueOK(); ok {
			if ws, ok := w.StringValueOK(); !ok || gs != ws {
				return false
			}
			continue
		}
		gf, gok := g.AsFloat64OK()
		wf, wok := w.AsFloat64OK()
		if !gok || !wok || gf != wf {
			return false
		}
	}
	return true
}

// namedModels returns copies of models carrying their resolved names in options of their own: the driver writes the name into a
// model's options builder, which would race when callers share one model slice across goroutines (a package-level var).
func namedModels(models []mongo.IndexModel, names []string) []mongo.IndexModel {
	out := make([]mongo.IndexModel, len(models))
	for i, model := range models {
		opts := mongoOptions.Index()
		if model.Options != nil {
			opts.Opts = slices.Clone(model.Options.Opts)
		}
		out[i] = mongo.IndexModel{Keys: model.Keys, Options: opts.SetName(names[i])}
	}
	return out
}

// isIndexConflict reports whether err is IndexOptionsConflict or IndexKeySpecsConflict.
func isIndexConflict(err error) bool {
	se, ok := errors.AsType[mongo.ServerError](err)
	return ok && (se.HasErrorCode(MongoErrorCodeIndexOptionsConflict) || se.HasErrorCode(MongoErrorCodeIndexKeySpecsConflict))
}

// dropIndexIfExists drops index name of coll; a missing collection or index is not an error.
func dropIndexIfExists(ctx context.Context, coll *mongo.Collection, name string) error {
	err := coll.Indexes().DropOne(ctx, name)
	if err == nil || IsErrorIndexNotFound(err) || IsErrorCollectionNotFound(err) {
		return nil
	}
	return coreerrs.WrapOperationWithContext(err, "drop index", coll.Name()+"."+name)
}

// indexModelNames resolves the name of every model and checks they are distinct.
func indexModelNames(models []mongo.IndexModel) ([]string, error) {
	names := make([]string, len(models))
	for i, model := range models {
		name, err := indexModelName(model)
		if err != nil {
			return nil, err
		}
		if slices.Contains(names[:i], name) {
			return nil, fmt.Errorf("%w: duplicate index name %q", ErrInvalidIndexModels, name)
		}
		names[i] = name
	}
	return names, nil
}

// indexModelName is the model's explicit name, or the one the driver generates: every key and its value (an integer or a string)
// joined by "_".
func indexModelName(model mongo.IndexModel) (string, error) {
	args, err := resolveIndexOptions(model)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidIndexModels, err)
	}
	if args.Name != nil {
		if *args.Name == "" {
			return "", fmt.Errorf("%w: empty index name", ErrInvalidIndexModels)
		}
		return *args.Name, nil
	}
	keys, err := bson.Marshal(model.Keys)
	if err != nil {
		return "", fmt.Errorf("%w: keys: %w", ErrInvalidIndexModels, err)
	}
	elems, err := bson.Raw(keys).Elements()
	if err != nil || len(elems) == 0 {
		return "", fmt.Errorf("%w: index keys must be a non-empty document", ErrInvalidIndexModels)
	}
	parts := make([]string, 0, 2*len(elems))
	for _, e := range elems {
		v := e.Value()
		var s string
		switch v.Type {
		case bson.TypeInt32:
			s = strconv.FormatInt(int64(v.Int32()), 10)
		case bson.TypeInt64:
			s = strconv.FormatInt(v.Int64(), 10)
		case bson.TypeString:
			s = v.StringValue()
		default:
			return "", fmt.Errorf("%w: key %q has a %v value", ErrInvalidIndexModels, e.Key(), v.Type)
		}
		parts = append(parts, e.Key(), s)
	}
	return strings.Join(parts, "_"), nil
}
