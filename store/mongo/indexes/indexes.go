// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package indexes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/store/mongo/internal/databaselock"

	coremaps "github.com/altessa-s/go-atlas-core/collections/maps"
	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	mongoxindexes "github.com/altessa-s/go-mongox/indexes"
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

// ErrInvalidIndexModels is returned by [Ensure] for models whose names collide, an obsolete name that is also a model's or
// empty, an attempt to drop all indexes or the _id index, or keys a name cannot be derived from.
var ErrInvalidIndexModels = mongoxindexes.ErrInvalidIndexModels

// Ensure makes the indexes of coll match models; it is idempotent and safe to run at every start. Without options it is one
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
// compound keys must use an ordered document such as bson.D,
// and an obsolete name may not be a model's, empty, "*", or "_id_" ([ErrInvalidIndexModels]).
func Ensure(ctx context.Context, coll *mongo.Collection, models []mongo.IndexModel, opts ...Option) error {
	for i, model := range models {
		keys := reflect.ValueOf(model.Keys)
		if keys.Kind() == reflect.Map && keys.Len() > 1 {
			return fmt.Errorf("%w: model %d has unordered compound index keys", ErrInvalidIndexModels, i)
		}
	}
	o := newOptions(opts...)
	if len(o.dropObsolete) == 0 && !o.dropOnSpecMismatch {
		return mongoxindexes.Ensure(ctx, coll, models)
	}

	// Reconciliation resolves names, compares specifications and may create a model more than once. Freeze
	// keys and option setters so every step uses the model validated before the first drop.
	models, err := freezeModels(models)
	if err != nil {
		return err
	}
	names, err := mongoxindexes.Names(models)
	if err != nil {
		return err
	}
	for _, name := range o.dropObsolete {
		switch {
		case name == "*":
			return errors.Join(ErrInvalidIndexModels, mongo.ErrMultipleIndexDrop)
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
	err = mongoxindexes.Ensure(ctx, coll, models)
	if err == nil || !o.dropOnSpecMismatch || !isIndexConflict(err) {
		return err
	}
	return recreateMismatched(ctx, coll, models, names)
}

// Index reconciliation lock: [WithDropOnSpecMismatch] drops and recreates indexes only while holding a per-collection mutex
// document in [databaselock.CollectionName], so replicas starting together cannot drop each other's freshly built index.
const (
	// IndexLockLease is the lease of the reconciliation lock; it bounds how long a crashed holder blocks the others and must
	// cover the index builds of one reconciliation.
	IndexLockLease = 2 * time.Minute
	// IndexLockWaitTimeout bounds how long Ensure waits for a peer's reconciliation of the same collection.
	IndexLockWaitTimeout = IndexLockLease + 30*time.Second

	indexLockPrefix = "indexes:"
)

// recreateMismatched reconciles the models under the collection's reconciliation lock: each model is created afresh, and only
// when that fails with a conflict while an index of the model's name exists whose current specification differs from the model is
// that index dropped and the model created again. A peer that already replaced the index makes the fresh create succeed, so a
// correct index is never dropped on the strength of a stale conflict.
func recreateMismatched(ctx context.Context, coll *mongo.Collection, models []mongo.IndexModel, names []string) error {
	locks := coll.Database().Collection(databaselock.CollectionName)
	id := indexLockPrefix + coll.Name()
	owner := bson.NewObjectID()

	acquireCtx, cancel := corecontext.WithMaxTimeout(ctx, IndexLockWaitTimeout)
	defer cancel()
	if err := databaselock.Acquire(acquireCtx, locks, id, owner, IndexLockLease); err != nil {
		return err
	}
	defer func() { _ = databaselock.Release(ctx, locks, id, owner) }() //nolint:errcheck // best effort: the lease bounds a failed release

	// The lease is renewed until the work returns; the work context keeps the IndexLockLease budget and is canceled only when a
	// renewal fails.
	//nolint:contextcheck // the lease carries the work and scope contexts
	return databaselock.Hold(ctx, locks, id, owner, IndexLockLease, IndexLockLease, func(l *databaselock.Lease) error {
		for i, model := range models {
			if err := reconcileIndex(l.Work(), coll, model, names[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

// reconcileIndex creates model (named name), replacing an existing index of that name whose specification differs.
func reconcileIndex(ctx context.Context, coll *mongo.Collection, model mongo.IndexModel, name string) error {
	err := mongoxindexes.Ensure(ctx, coll, []mongo.IndexModel{model})
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
	if err := mongoxindexes.Ensure(ctx, coll, []mongo.IndexModel{model}); err != nil {
		return coreerrs.WrapOperationWithContext(err, "recreate mismatched index", coll.Name()+"."+name)
	}
	return nil
}

// indexSpecification returns the full listIndexes document of index name of coll.
func indexSpecification(ctx context.Context, coll *mongo.Collection, name string) (bson.Raw, bool, error) {
	cur, err := coll.Indexes().List(ctx)
	if err != nil {
		if hasServerCode(err, namespaceNotFound) {
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
	if err != nil || hasUncomparedOptions(args) || !onlyComparedFields(spec) {
		return false
	}
	keys, err := bson.Marshal(model.Keys)
	if err != nil || !sameKeys(spec.Lookup("key"), keys) {
		return false
	}
	return sameIndexOptions(spec, args)
}

// hasUncomparedOptions reports whether the model sets an index option that
// indexMatchesModel does not compare.
func hasUncomparedOptions(args mongoOptions.IndexOptions) bool {
	return args.Collation != nil || args.Weights != nil || args.DefaultLanguage != nil || args.LanguageOverride != nil ||
		args.TextVersion != nil || args.SphereVersion != nil || args.Bits != nil || args.Max != nil || args.Min != nil ||
		args.StorageEngine != nil || args.WildcardProjection != nil
}

// onlyComparedFields reports whether every field of the stored specification
// is one indexMatchesModel compares.
func onlyComparedFields(spec bson.Raw) bool {
	elems, err := spec.Elements()
	if err != nil {
		return false
	}
	for _, e := range elems {
		if !indexComparedOptions.Contains(e.Key()) {
			return false
		}
	}
	return true
}

// sameIndexOptions reports whether the stored unique, sparse, hidden,
// expireAfterSeconds and partialFilterExpression equal the model's.
func sameIndexOptions(spec bson.Raw, args mongoOptions.IndexOptions) bool {
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

// freezeModels snapshots keys and option setters for destructive reconciliation.
func freezeModels(models []mongo.IndexModel) ([]mongo.IndexModel, error) {
	out := make([]mongo.IndexModel, len(models))
	for i, model := range models {
		keys, err := bson.Marshal(model.Keys)
		if err != nil {
			return nil, errors.Join(ErrInvalidIndexModels, err)
		}
		args, err := resolveIndexOptions(model)
		if err != nil {
			return nil, errors.Join(ErrInvalidIndexModels, err)
		}
		opts := mongoOptions.Index()
		opts.Opts = append(opts.Opts, func(o *mongoOptions.IndexOptions) error {
			*o = args
			return nil
		})
		out[i] = mongo.IndexModel{Keys: bson.Raw(keys), Options: opts}
	}
	return out, nil
}

// isIndexConflict reports whether err is IndexOptionsConflict or IndexKeySpecsConflict.
func isIndexConflict(err error) bool {
	se, ok := errors.AsType[mongo.ServerError](err)
	return ok && (se.HasErrorCode(MongoErrorCodeIndexOptionsConflict) || se.HasErrorCode(MongoErrorCodeIndexKeySpecsConflict))
}

// dropIndexIfExists drops index name of coll; a missing collection or index is not an error.
func dropIndexIfExists(ctx context.Context, coll *mongo.Collection, name string) error {
	err := coll.Indexes().DropOne(ctx, name)
	if err == nil || hasServerCode(err, indexNotFound) || hasServerCode(err, namespaceNotFound) {
		return nil
	}
	return coreerrs.WrapOperationWithContext(err, "drop index", coll.Name()+"."+name)
}

const (
	namespaceNotFound = 26
	indexNotFound     = 27
)

func hasServerCode(err error, code int) bool {
	se, ok := errors.AsType[mongo.ServerError](err)
	return ok && se.HasErrorCode(code)
}
