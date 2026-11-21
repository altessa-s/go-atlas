// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keyset

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/altessa-s/go-aip/pagination/pagetoken"
	"github.com/altessa-s/go-mongox/bsonfilter"

	"github.com/altessa-s/go-atlas/store/mongo/internal/query"

	mongokeyset "github.com/altessa-s/go-mongox/keyset"
)

const (
	// DefaultPageSize is the page size [FindPage] uses when the query
	// limit is not positive.
	DefaultPageSize = mongokeyset.DefaultPageSize
	// MaxPageSize caps the page size [FindPage] reads.
	MaxPageSize = mongokeyset.MaxPageSize
)

var (
	// ErrFilterContainsDangerousOperator identifies a filter containing a server-side JavaScript operator.
	ErrFilterContainsDangerousOperator = bsonfilter.ErrDangerousOperator
	// ErrSort is returned by [FindPage] for a sort it cannot page: a
	// direction other than 1 or -1, an empty or repeated field.
	ErrSort = mongokeyset.ErrSort
	// ErrMissingKey is returned by [FindPage] when a returned document
	// lacks a sort field, or holds null or an array there — keyset paging
	// needs a comparable scalar in every sort field of every document, and
	// the projection must keep the sort fields.
	ErrMissingKey = mongokeyset.ErrMissingKey
	// ErrProjection is returned by [FindPage] for a projection that
	// drops or computes a sort field: the page position is
	// read from the returned document and must be the stored value.
	ErrProjection = mongokeyset.ErrProjection
	// ErrTokensRequired is returned by [FindPage] for a query with a
	// page token but no codec. It wraps [pagetoken.ErrCodecRequired], so
	// [pagetoken.Classify] reports it as a malformed token.
	ErrTokensRequired = mongokeyset.ErrTokensRequired
)

// Query describes one page of a [FindPage] read.
type Query struct {
	// TimestampPrecision requires stored cursor dates to align to this resolution.
	// Zero uses BSON milliseconds. It is included in token bindings.
	TimestampPrecision time.Duration

	// Filter is an ordered query predicate; nil matches every document. The same BSON bytes bind the token
	// and execute the find. Nested bson.D and bson.Raw retain field order; bson.M and map[string]any
	// keys are sorted. Use bson.D or pre-built bson.Raw for embedded documents and admit custom scalar
	// struct codecs with go-mongox/bsonfilter.AdmitScalar on the registry used for this query.
	Filter bson.D
	// Registry encodes Filter. Nil uses the Atlas client's registry, or the Atlas default for an external client.
	// Pass the effective registry explicitly for external clients or database or collection overrides.
	Registry *bson.Registry
	// BSONOptions configures filter encoding. Nil inherits the Atlas client's settings or driver defaults for external clients.
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
	q.Registry, q.BSONOptions = query.FilterEncoding(coll, q.Registry, q.BSONOptions)
	page, err := mongokeyset.FindPage[T](ctx, coll, codec, mongokeyset.Query(q))
	if err == mongokeyset.ErrPrecision { // Preserve the Atlas error for an invalid query precision.
		return Page[T]{}, fmt.Errorf("%w: %w", ErrMissingKey, err)
	}
	return Page[T](page), err
}
