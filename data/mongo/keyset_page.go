// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/altessa-s/go-atlas/data/keyset"
)

const (
	// DefaultKeysetPageSize is the page size [FindPage] uses when the query
	// limit is not positive.
	DefaultKeysetPageSize = 100
	// MaxKeysetPageSize caps the page size [FindPage] reads.
	MaxKeysetPageSize = 10_000
)

var (
	// ErrKeysetSort is returned by [FindPage] for a sort it cannot page: a
	// direction other than 1 or -1, an empty or repeated field.
	ErrKeysetSort = errors.New("mongo: invalid keyset sort")
	// ErrKeysetMissingKey is returned by [FindPage] when a returned document
	// lacks a sort field, or holds null or an array there — keyset paging
	// needs a comparable scalar in every sort field of every document, and
	// the projection must keep the sort fields.
	ErrKeysetMissingKey = errors.New("mongo: document lacks a keyset sort value")
	// ErrKeysetProjection is returned by [FindPage] for a projection that
	// computes a sort field or a field containing one: the page position is
	// read from the returned document and must be the stored value. Plain
	// inclusion (1, true) and exclusion (0, false) are allowed.
	ErrKeysetProjection = errors.New("mongo: projection rewrites a keyset sort field")
	// ErrKeysetTokensRequired is returned by [FindPage] for a query with a
	// page token but no codec. It wraps [keyset.ErrCodecRequired], so
	// [keyset.Classify] reports it as a malformed token.
	ErrKeysetTokensRequired = fmt.Errorf("mongo: %w", keyset.ErrCodecRequired)
)

// KeysetQuery describes one page of a [FindPage] read.
type KeysetQuery struct {
	// Filter is the query predicate; nil matches every document.
	Filter bson.D
	// Sort is the order, as [ParseSortString] or the data/orderby Mongo
	// translator produce it: field names with 1 (ascending) or -1
	// (descending). "_id" ascending is appended as the tie-breaker when the
	// sort does not end the order on it.
	Sort bson.D
	// Projection limits the returned fields; it must keep the sort fields.
	Projection bson.D
	// Limit is the page size: DefaultKeysetPageSize when not positive, at
	// most MaxKeysetPageSize.
	Limit int64
	// After is the token of the page to read; empty reads the first page.
	After string
	// Subject is the principal the token is bound to.
	Subject string
}

// KeysetPage is one page of a [FindPage] read.
type KeysetPage[T any] struct {
	// Items are the documents of the page in sort order.
	Items []T
	// Next is the token of the following page, empty on the last page.
	Next string
}

// keysetKey is one sort key of a keyset read.
type keysetKey struct {
	field string
	desc  bool
}

// FindPage reads one page of coll with a plain find, positioned by a signed
// page token rather than an offset. Unlike [ListCursor] it works with any
// "_id" type (ObjectID, string, UUID), needs no created_at field, and
// streams documents instead of collecting them into one aggregation result,
// so the 16 MB document limit does not apply to a page.
//
// The token carries the sort values of the last document of a page and is
// bound with codec to the filter, the sort and the subject: replaying it
// under another filter, sort or principal fails with the [keyset] errors. A
// nil codec reads a single page and issues no Next token; a page token then
// fails with [ErrKeysetTokensRequired]. Every document must hold a comparable
// scalar of one BSON type in each sort field (see [ErrKeysetMissingKey]); an
// index on the sort fields keeps each page a range scan. Continuation is
// keyset-based, not a snapshot: documents inserted ahead of the position
// appear on later pages. Documents are decoded with the collection's
// registry.
func FindPage[T any](ctx context.Context, coll *mongo.Collection, codec *keyset.Codec, q KeysetQuery) (KeysetPage[T], error) {
	keys, err := keysetKeys(q.Sort)
	if err != nil {
		return KeysetPage[T]{}, err
	}
	if err = checkKeysetProjection(q.Projection, keys); err != nil {
		return KeysetPage[T]{}, err
	}
	bindings, err := keysetBindings(q, keys)
	if err != nil {
		return KeysetPage[T]{}, err
	}

	filter := q.Filter
	if q.After != "" {
		if codec == nil {
			return KeysetPage[T]{}, ErrKeysetTokensRequired
		}
		payload, resolveErr := codec.Resolve(q.After, bindings)
		if resolveErr != nil {
			return KeysetPage[T]{}, fmt.Errorf("mongo: decode page token: %w", resolveErr)
		}
		values, decodeErr := decodeKeysetPosition(payload, len(keys))
		if decodeErr != nil {
			return KeysetPage[T]{}, decodeErr
		}
		filter = withKeysetSeek(filter, keys, values)
	}
	if filter == nil {
		filter = bson.D{}
	}

	limit := int64(keyset.PageSize(q.Limit, DefaultKeysetPageSize, MaxKeysetPageSize))

	sort := make(bson.D, 0, len(keys))
	for _, k := range keys {
		sort = append(sort, bson.E{Key: k.field, Value: keysetDirection(k.desc)})
	}
	// One batch holds the whole page plus the look-ahead document, so a page
	// beyond the server's default first batch (101) needs no getMore.
	opts := options.Find().SetSort(sort).SetLimit(limit + 1).SetBatchSize(int32(limit + 1)) //nolint:gosec // limit <= MaxKeysetPageSize
	if len(q.Projection) > 0 {
		opts.SetProjection(q.Projection)
	}

	cur, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return KeysetPage[T]{}, fmt.Errorf("mongo: find page: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }() //nolint:errcheck // read-only cursor

	page := KeysetPage[T]{Items: make([]T, 0, min(limit, DefaultKeysetPageSize))}
	var last bson.Raw
	more := false
	for cur.Next(ctx) {
		if int64(len(page.Items)) == limit {
			more = true
			break
		}
		var item T
		if err = cur.Decode(&item); err != nil {
			return KeysetPage[T]{}, fmt.Errorf("mongo: decode page document: %w", err)
		}
		page.Items = append(page.Items, item)
		last = cur.Current
	}
	if err = cur.Err(); err != nil {
		return KeysetPage[T]{}, fmt.Errorf("mongo: read page: %w", err)
	}
	if !more || codec == nil {
		return page, nil
	}

	payload, err := encodeKeysetPosition(last, keys)
	if err != nil {
		return KeysetPage[T]{}, err
	}
	if page.Next, err = codec.Issue(payload, bindings); err != nil {
		return KeysetPage[T]{}, fmt.Errorf("mongo: encode page token: %w", err)
	}
	return page, nil
}

// keysetKeys validates sort and returns its keys, with "_id" ascending
// appended unless the sort already ends on "_id".
func keysetKeys(sort bson.D) ([]keysetKey, error) {
	keys := make([]keysetKey, 0, len(sort)+1)
	seen := make(map[string]struct{}, len(sort))
	for _, e := range sort {
		if e.Key == "" || strings.HasPrefix(e.Key, "$") {
			return nil, fmt.Errorf("%w: field %q", ErrKeysetSort, e.Key)
		}
		if _, dup := seen[e.Key]; dup {
			return nil, fmt.Errorf("%w: field %q repeated", ErrKeysetSort, e.Key)
		}
		seen[e.Key] = struct{}{}
		desc, err := keysetDesc(e.Value)
		if err != nil {
			return nil, fmt.Errorf("%w: field %q: %w", ErrKeysetSort, e.Key, err)
		}
		keys = append(keys, keysetKey{field: e.Key, desc: desc})
		if e.Key == "_id" {
			// _id is unique: later keys can never break a tie.
			return keys, nil
		}
	}
	return append(keys, keysetKey{field: "_id"}), nil
}

// checkKeysetProjection rejects a projection entry that touches a sort field
// — the field itself, an ancestor or a descendant of it — with anything but
// plain inclusion or exclusion ($literal, $slice, $elemMatch, an expression,
// a positional "field.$").
func checkKeysetProjection(projection bson.D, keys []keysetKey) error {
	for _, e := range projection {
		path := strings.TrimSuffix(e.Key, ".$")
		for _, k := range keys {
			related := path == k.field || strings.HasPrefix(k.field, path+".") || strings.HasPrefix(path, k.field+".")
			if !related {
				continue
			}
			if path != e.Key || !isPlainProjection(e.Value) {
				return fmt.Errorf("%w: %q", ErrKeysetProjection, e.Key)
			}
		}
	}
	return nil
}

// isPlainProjection reports whether v is a plain inclusion or exclusion.
func isPlainProjection(v any) bool {
	switch x := v.(type) {
	case bool:
		return true
	case int:
		return x == 0 || x == 1
	case int32:
		return x == 0 || x == 1
	case int64:
		return x == 0 || x == 1
	case float64:
		return x == 0 || x == 1
	default:
		return false
	}
}

// keysetDesc reports whether a sort direction is descending.
func keysetDesc(v any) (bool, error) {
	var n int64
	switch d := v.(type) {
	case int:
		n = int64(d)
	case int32:
		n = int64(d)
	case int64:
		n = d
	default:
		return false, fmt.Errorf("direction %v is not 1 or -1", v)
	}
	switch n {
	case 1:
		return false, nil
	case -1:
		return true, nil
	default:
		return false, fmt.Errorf("direction %d is not 1 or -1", n)
	}
}

func keysetDirection(desc bool) int32 {
	if desc {
		return -1
	}
	return 1
}

// keysetBindings binds a token to the effective sort and the filter.
func keysetBindings(q KeysetQuery, keys []keysetKey) (keyset.Bindings, error) {
	var sortFP strings.Builder
	for _, k := range keys {
		sortFP.WriteString(strconv.Quote(k.field))
		if k.desc {
			sortFP.WriteString(":-1;")
		} else {
			sortFP.WriteString(":1;")
		}
	}
	filter := q.Filter
	if filter == nil {
		filter = bson.D{}
	}
	// BSON bytes, not Extended JSON: they keep every type tag, so an int32
	// and a {"$numberInt": …} document cannot share a fingerprint.
	filterFP, err := bson.Marshal(bson.D{{Key: "f", Value: filter}})
	if err != nil {
		return keyset.Bindings{}, fmt.Errorf("mongo: fingerprint page filter: %w", err)
	}
	return keyset.Bindings{Sort: sortFP.String(), Filter: string(filterFP), Subject: q.Subject}, nil
}

// encodeKeysetPosition returns the sort values of doc as a BSON array
// document, the token payload.
func encodeKeysetPosition(doc bson.Raw, keys []keysetKey) ([]byte, error) {
	values := make(bson.A, 0, len(keys))
	for _, k := range keys {
		v, err := doc.LookupErr(strings.Split(k.field, ".")...)
		if err != nil || v.Type == bson.TypeNull || v.Type == bson.TypeArray || v.Type == bson.TypeUndefined {
			return nil, fmt.Errorf("%w: %q", ErrKeysetMissingKey, k.field)
		}
		values = append(values, v)
	}
	payload, err := bson.Marshal(bson.D{{Key: "k", Value: values}})
	if err != nil {
		return nil, fmt.Errorf("mongo: encode page position: %w", err)
	}
	return payload, nil
}

// decodeKeysetPosition parses a token payload into n sort values.
func decodeKeysetPosition(payload []byte, n int) ([]bson.RawValue, error) {
	invalid := fmt.Errorf("mongo: decode page token: %w", keyset.ErrInvalidToken)
	raw := bson.Raw(payload)
	if raw.Validate() != nil {
		return nil, invalid
	}
	arr, ok := raw.Lookup("k").ArrayOK()
	if !ok {
		return nil, invalid
	}
	values, err := arr.Values()
	if err != nil || len(values) != n {
		return nil, invalid
	}
	return values, nil
}

// withKeysetSeek restricts filter to the documents after the position
// values in the order of keys: (k1 > v1) or (k1 = v1 and k2 > v2) or …, with
// $lt for descending keys.
func withKeysetSeek(filter bson.D, keys []keysetKey, values []bson.RawValue) bson.D {
	branches := make(bson.A, 0, len(keys))
	for i, k := range keys {
		branch := make(bson.D, 0, i+1)
		for j := range i {
			branch = append(branch, bson.E{Key: keys[j].field, Value: bson.D{{Key: "$eq", Value: values[j]}}})
		}
		op := "$gt"
		if k.desc {
			op = "$lt"
		}
		branch = append(branch, bson.E{Key: k.field, Value: bson.D{{Key: op, Value: values[i]}}})
		branches = append(branches, branch)
	}
	seek := bson.D{{Key: "$or", Value: branches}}
	if len(filter) == 0 {
		return seek
	}
	return bson.D{{Key: "$and", Value: bson.A{filter, seek}}}
}
