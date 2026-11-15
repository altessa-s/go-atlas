// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keyset

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/altessa-s/go-aip/pagination/pagetoken"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/store/mongo/internal/query"
)

const (
	// DefaultPageSize is the page size [FindPage] uses when the query
	// limit is not positive.
	DefaultPageSize = 100
	// MaxPageSize caps the page size [FindPage] reads.
	MaxPageSize = 10_000
)

var (
	// ErrFilterContainsDangerousOperator identifies a filter containing a server-side JavaScript operator.
	ErrFilterContainsDangerousOperator = query.ErrDangerousOperator
	// ErrSort is returned by [FindPage] for a sort it cannot page: a
	// direction other than 1 or -1, an empty or repeated field.
	ErrSort = errors.New("mongo: invalid keyset sort")
	// ErrMissingKey is returned by [FindPage] when a returned document
	// lacks a sort field, or holds null or an array there — keyset paging
	// needs a comparable scalar in every sort field of every document, and
	// the projection must keep the sort fields.
	ErrMissingKey = errors.New("mongo: document lacks a keyset sort value")
	// ErrProjection is returned by [FindPage] for a projection that
	// computes a sort field or a field containing one: the page position is
	// read from the returned document and must be the stored value. Plain
	// inclusion (1, true) and exclusion (0, false) are allowed.
	ErrProjection = errors.New("mongo: projection rewrites a keyset sort field")
	// ErrTokensRequired is returned by [FindPage] for a query with a
	// page token but no codec. It wraps [pagetoken.ErrCodecRequired], so
	// [pagetoken.Classify] reports it as a malformed token.
	ErrTokensRequired = fmt.Errorf("mongo: %w", pagetoken.ErrCodecRequired)
)

// Query describes one page of a [FindPage] read.
type Query struct {
	// TimestampPrecision requires stored cursor dates to align to this resolution.
	// Zero uses BSON milliseconds. It is included in token bindings.
	TimestampPrecision time.Duration

	// Filter is the query predicate; nil matches every document. It is marshaled once, with Registry and BSONOptions: the page
	// token is bound to those bytes and the same bytes are sent to the server. Build it deterministically — bson.D documents, not
	// multi-key bson.M maps, whose iteration order varies — or a token issued for one request does not resolve for the identical
	// next one; the go-aip backend/mongo filter translator's TranslateD output qualifies. An embedded-document literal keeps its key order,
	// which MongoDB compares.
	Filter bson.D
	// Registry encodes Filter. Nil uses the go-atlas codec registry the Mongo wrapper installs (store/mongo/bsoncodec.NewRegistry), so
	// decimal, date and Optional values encode as for the collection; pass the collection's registry when its client, database or
	// collection was configured with another one. The collection's own registry is not used: the driver does not expose it.
	Registry *bson.Registry
	// BSONOptions are the encoder settings of the collection (options.Collection().SetBSONOptions); nil uses the defaults.
	BSONOptions *options.BSONOptions
	// Sort is the order, as the go-aip backend/mongo order
	// translator produces it: field names with 1 (ascending) or -1
	// (descending). TiebreakPath ascending is appended when absent; an empty
	// TiebreakPath selects "_id".
	Sort bson.D
	// TiebreakPath is the unique final sort path; empty uses "_id".
	TiebreakPath string
	// Projection limits the returned fields; it must keep the sort fields.
	Projection bson.D
	// Limit is the page size: DefaultPageSize when not positive, at
	// most MaxPageSize.
	Limit int64
	// After is the token of the page to read; empty reads the first page.
	After string
	// Subject is the principal the token is bound to.
	Subject string
}

// Page is one page of a [FindPage] read.
type Page[T any] struct {
	// Items are the documents of the page in sort order.
	Items []T
	// Next is the token of the following page, empty on the last page.
	Next string
}

// TokenCodec issues and resolves bound page tokens.
type TokenCodec interface {
	Issue(payload []byte, b pagetoken.Bindings) (string, error)
	Resolve(token string, b pagetoken.Bindings) ([]byte, error)
}

// sortKey is one sort key of a keyset read.
type sortKey struct {
	field string
	desc  bool
}

// FindPage reads one page of coll with a plain find, positioned by a signed
// page token rather than an offset. Unlike mongo.ListCursor it works with any
// "_id" type (ObjectID, string, UUID), needs no created_at field, and
// streams documents instead of collecting them into one aggregation result,
// so the 16 MB document limit does not apply to a page.
//
// The token carries the sort values of the last document of a page and is
// bound with codec to the filter, the sort and the subject: replaying it
// under another filter, sort or principal fails with the [pagetoken] errors. A
// nil codec reads a single page and issues no Next token; a page token then
// fails with [ErrTokensRequired]. Every document must hold a comparable
// scalar of one BSON type in each sort field (see [ErrMissingKey]); an
// index on the sort fields keeps each page a range scan. Continuation is
// keyset-based, not a snapshot: documents inserted ahead of the position
// appear on later pages. Documents are decoded with the collection's
// registry.
// The caller context owns the query deadline; FindPage adds no timeout.
func FindPage[T any](ctx context.Context, coll *mongo.Collection, codec TokenCodec, q Query) (Page[T], error) {
	if nilcheck.IsNil(codec) {
		codec = nil
	}
	if err := query.ValidateFilter(q.Filter); err != nil {
		return Page[T]{}, err
	}
	if q.TimestampPrecision == 0 {
		q.TimestampPrecision = time.Millisecond
	}
	if q.TimestampPrecision < time.Millisecond || q.TimestampPrecision%time.Millisecond != 0 {
		return Page[T]{}, fmt.Errorf("%w: invalid timestamp precision", ErrMissingKey)
	}
	keys, err := sortKeys(q.Sort, q.TiebreakPath)
	if err != nil {
		return Page[T]{}, err
	}
	if err = checkProjection(q.Projection, keys); err != nil {
		return Page[T]{}, err
	}
	filter, err := marshalFilter(q.Filter, q.Registry, q.BSONOptions)
	if err != nil {
		return Page[T]{}, err
	}
	bindings, err := newBindings(filter, keys, q.Subject+"/precision="+q.TimestampPrecision.String())
	if err != nil {
		return Page[T]{}, err
	}

	var predicate any = filter
	if q.After != "" {
		if codec == nil {
			return Page[T]{}, ErrTokensRequired
		}
		payload, resolveErr := codec.Resolve(q.After, bindings)
		if resolveErr != nil {
			return Page[T]{}, fmt.Errorf("mongo: decode page token: %w", resolveErr)
		}
		values, decodeErr := decodePosition(payload, len(keys))
		if decodeErr != nil {
			return Page[T]{}, decodeErr
		}
		predicate = withSeek(filter, keys, values)
	}

	limit := int64(pagetoken.PageSize(q.Limit, DefaultPageSize, MaxPageSize))

	sort := make(bson.D, 0, len(keys))
	for _, k := range keys {
		sort = append(sort, bson.E{Key: k.field, Value: sortDirection(k.desc)})
	}
	// One batch holds the whole page plus the look-ahead document, so a page
	// beyond the server's default first batch (101) needs no getMore.
	opts := options.Find().SetSort(sort).SetLimit(limit + 1).SetBatchSize(int32(limit + 1)) //nolint:gosec // limit <= MaxPageSize
	if len(q.Projection) > 0 {
		opts.SetProjection(q.Projection)
	}

	cur, err := coll.Find(ctx, predicate, opts)
	if err != nil {
		return Page[T]{}, fmt.Errorf("mongo: find page: %w", err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.WithoutCancel(ctx), query.CloseTimeout)
		defer closeCancel()
		_ = cur.Close(closeCtx)
	}() //nolint:errcheck // read-only cursor

	page := Page[T]{Items: make([]T, 0, min(limit, DefaultPageSize))}
	var last bson.Raw
	more := false
	for cur.Next(ctx) {
		if int64(len(page.Items)) == limit {
			more = true
			break
		}
		var item T
		if err = cur.Decode(&item); err != nil {
			return Page[T]{}, fmt.Errorf("mongo: decode page document: %w", err)
		}
		page.Items = append(page.Items, item)
		last = cur.Current
	}
	if err = cur.Err(); err != nil {
		return Page[T]{}, fmt.Errorf("mongo: read page: %w", err)
	}
	if !more || codec == nil {
		return page, nil
	}

	payload, err := encodePositionPrecision(last, keys, q.TimestampPrecision)
	if err != nil {
		return Page[T]{}, err
	}
	if page.Next, err = codec.Issue(payload, bindings); err != nil {
		return Page[T]{}, fmt.Errorf("mongo: encode page token: %w", err)
	}
	return page, nil
}

// sortKeys validates sort and ends it on the configured unique identity,
// appending that identity ascending when absent.
func sortKeys(sort bson.D, tiebreakPath string) ([]sortKey, error) {
	if tiebreakPath == "" {
		tiebreakPath = "_id"
	}
	if strings.HasPrefix(tiebreakPath, "$") || strings.Contains(tiebreakPath, "\x00") {
		return nil, fmt.Errorf("%w: tie-breaker path %q", ErrSort, tiebreakPath)
	}
	keys := make([]sortKey, 0, len(sort)+1)
	seen := make(map[string]struct{}, len(sort))
	for _, e := range sort {
		if e.Key == "" || strings.HasPrefix(e.Key, "$") {
			return nil, fmt.Errorf("%w: field %q", ErrSort, e.Key)
		}
		if _, dup := seen[e.Key]; dup {
			return nil, fmt.Errorf("%w: field %q repeated", ErrSort, e.Key)
		}
		seen[e.Key] = struct{}{}
		desc, err := sortDescending(e.Value)
		if err != nil {
			return nil, fmt.Errorf("%w: field %q: %w", ErrSort, e.Key, err)
		}
		keys = append(keys, sortKey{field: e.Key, desc: desc})
		if e.Key == tiebreakPath {
			// The configured identity is unique: later keys cannot break a tie.
			return keys, nil
		}
	}
	return append(keys, sortKey{field: tiebreakPath}), nil
}

// checkProjection rejects a projection entry that touches a sort field
// — the field itself, an ancestor or a descendant of it — with anything but
// plain inclusion or exclusion ($literal, $slice, $elemMatch, an expression,
// a positional "field.$").
func checkProjection(projection bson.D, keys []sortKey) error {
	for _, e := range projection {
		path := strings.TrimSuffix(e.Key, ".$")
		for _, k := range keys {
			related := path == k.field || strings.HasPrefix(k.field, path+".") || strings.HasPrefix(path, k.field+".")
			if !related {
				continue
			}
			if path != e.Key || !isPlainProjection(e.Value) {
				return fmt.Errorf("%w: %q", ErrProjection, e.Key)
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

// sortDescending reports whether a sort direction is descending.
func sortDescending(v any) (bool, error) {
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

func sortDirection(desc bool) int32 {
	if desc {
		return -1
	}
	return 1
}

// marshalFilter marshals the query filter once (nil as the empty document) with reg (nil: the registry the Mongo wrapper installs)
// and the encoder settings of opts, as the driver encodes a filter: FindPage binds the token to these bytes and sends them.
func marshalFilter(filter bson.D, reg *bson.Registry, opts *options.BSONOptions) (bson.Raw, error) {
	if filter == nil {
		filter = bson.D{}
	}
	if reg == nil {
		reg = query.Registry()
	}
	var buf bytes.Buffer
	enc := bson.NewEncoder(bson.NewDocumentWriter(&buf))
	enc.SetRegistry(reg)
	applyBSONOptions(enc, opts)
	if err := enc.Encode(filter); err != nil {
		return nil, fmt.Errorf("mongo: marshal page filter: %w", err)
	}
	return buf.Bytes(), nil
}

// applyBSONOptions configures enc with the encoder settings of opts, as the driver does for a collection.
func applyBSONOptions(enc *bson.Encoder, opts *options.BSONOptions) {
	if opts == nil {
		return
	}
	for _, o := range []struct {
		on    bool
		apply func()
	}{
		{opts.ErrorOnInlineDuplicates, enc.ErrorOnInlineDuplicates},
		{opts.IntMinSize, enc.IntMinSize},
		{opts.NilByteSliceAsEmpty, enc.NilByteSliceAsEmpty},
		{opts.NilMapAsEmpty, enc.NilMapAsEmpty},
		{opts.NilSliceAsEmpty, enc.NilSliceAsEmpty},
		{opts.OmitZeroStruct, enc.OmitZeroStruct},
		{opts.OmitEmpty, enc.OmitEmpty},
		{opts.StringifyMapKeysWithFmt, enc.StringifyMapKeysWithFmt},
		{opts.UseJSONStructTags, enc.UseJSONStructTags},
	} {
		if o.on {
			o.apply()
		}
	}
}

// newBindings binds a token to the effective sort, the filter bytes FindPage executes and the subject.
func newBindings(filter bson.Raw, keys []sortKey, subject string) (pagetoken.Bindings, error) {
	var sortFP strings.Builder
	for _, k := range keys {
		sortFP.WriteString(strconv.Quote(k.field))
		if k.desc {
			sortFP.WriteString(":-1;")
		} else {
			sortFP.WriteString(":1;")
		}
	}
	// BSON bytes, not Extended JSON: they keep every type tag, so an int32
	// and a {"$numberInt": …} document cannot share a fingerprint.
	filterFP, err := bson.Marshal(bson.D{{Key: "f", Value: filter}})
	if err != nil {
		return pagetoken.Bindings{}, fmt.Errorf("mongo: fingerprint page filter: %w", err)
	}
	return pagetoken.Bindings{Sort: sortFP.String(), Filter: string(filterFP), Subject: subject}, nil
}

// encodePosition returns the sort values of doc as a BSON array
// document, the token payload.
func encodePosition(doc bson.Raw, keys []sortKey) ([]byte, error) {
	return encodePositionPrecision(doc, keys, time.Millisecond)
}

func encodePositionPrecision(doc bson.Raw, keys []sortKey, precision time.Duration) ([]byte, error) {
	values := make(bson.A, 0, len(keys))
	for _, k := range keys {
		v, err := doc.LookupErr(strings.Split(k.field, ".")...)
		if err != nil || v.Type == bson.TypeNull || v.Type == bson.TypeArray || v.Type == bson.TypeUndefined {
			return nil, fmt.Errorf("%w: %q", ErrMissingKey, k.field)
		}
		if v.Type == bson.TypeDateTime {
			at := time.UnixMilli(v.DateTime()).UTC()
			if !at.Equal(at.Truncate(precision)) {
				return nil, fmt.Errorf("%w: timestamp %q is not aligned to %s", ErrMissingKey, k.field, precision)
			}
		}
		values = append(values, v)
	}
	payload, err := bson.Marshal(bson.D{{Key: "k", Value: values}})
	if err != nil {
		return nil, fmt.Errorf("mongo: encode page position: %w", err)
	}
	return payload, nil
}

// decodePosition parses a token payload into n sort values.
func decodePosition(payload []byte, n int) ([]bson.RawValue, error) {
	invalid := fmt.Errorf("mongo: decode page token: %w", pagetoken.ErrInvalidToken)
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

// withSeek restricts filter, a marshaled document, to the documents
// after the position values in the order of keys: (k1 > v1) or (k1 = v1 and
// k2 > v2) or …, with $lt for descending keys.
func withSeek(filter bson.Raw, keys []sortKey, values []bson.RawValue) bson.D {
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
	if isEmptyDocument(filter) {
		return seek
	}
	return bson.D{{Key: "$and", Value: bson.A{filter, seek}}}
}

// emptyDocumentLength is the length of a marshaled empty BSON document: the int32 length and the terminating NUL.
const emptyDocumentLength = 5

// isEmptyDocument reports whether raw is absent or the empty document.
func isEmptyDocument(raw bson.Raw) bool { return len(raw) <= emptyDocumentLength }
