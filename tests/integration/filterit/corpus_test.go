// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filterit

import (
	"time"

	"github.com/altessa-s/go-atlas/data/filter"
)

// SQLFieldMapping maps the corpus field names onto the snake_case
// columns the SQL schemas use.
//
// `row.deletedAt` is there because CEL's has() macro only accepts a
// field selection — `has(deletedAt)` does not parse — so the corpus
// spells it `has(row.deletedAt)` and the mapping folds the qualifier
// away.
func SQLFieldMapping() map[string]string {
	return map[string]string{
		"createdAt":     "created_at",
		"deletedAt":     "deleted_at",
		"row.deletedAt": "deleted_at",
	}
}

// DocFieldMapping is the equivalent for the document and search
// backends, whose attributes keep the camelCase names verbatim.
func DocFieldMapping() map[string]string {
	return map[string]string{"row.deletedAt": "deletedAt"}
}

// Backend identifies one target of the shared corpus.
type Backend string

// The six backends the data/filter translators target.
const (
	ClickHouse Backend = "clickhouse"
	MariaDB    Backend = "mariadb"
	Postgres   Backend = "postgres"
	Mongo      Backend = "mongo"
	Meili      Backend = "meili"
	RediSearch Backend = "redisearch"
)

// Row is one record of the shared dataset. Every backend stores the same
// nine fields; how they are typed is the backend's business.
//
// DeletedAt is a pointer because the null cases need a column that is
// genuinely absent for most rows — it is the only nullable field, and the
// one the `== null` / `has()` cases key off.
type Row struct {
	DeletedAt *time.Time
	Name      string
	Status    string
	CreatedAt time.Time
	Price     float64
	ID        int64
	Age       int64
	Role      int64
	Active    bool
}

// Reference points for the dataset. Every timestamp below is a whole
// second in UTC so that a backend storing epoch seconds (Meilisearch) and
// one storing microseconds (PostgreSQL) agree on every comparison.
var (
	// Epoch is the base timestamp; CreatedAt values are offsets from it.
	Epoch = time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)

	deletedAt = Epoch.Add(240 * time.Hour)
)

// Dataset returns the seven rows every backend is loaded with.
//
// The values are chosen so that no single predicate selects everything or
// nothing: each comparison, each membership test and each string
// predicate splits the set. Names differ in case (Alice / alice) because
// that is where MariaDB's default collation parts ways with the others.
func Dataset() []Row {
	return []Row{
		{
			ID: 1, Name: "Alice", Age: 30, Price: 10.5, Active: true,
			Status: "active", Role: 1, CreatedAt: Epoch.Add(24 * time.Hour),
		},
		{
			ID: 2, Name: "Bob", Age: 25, Price: 20, Active: false,
			Status: "pending", Role: 2, CreatedAt: Epoch.Add(48 * time.Hour),
		},
		{
			ID: 3, Name: "Charlie", Age: 35, Price: 30.25, Active: true,
			Status: "active", Role: 1, CreatedAt: Epoch.Add(60 * time.Hour),
		},
		{
			ID: 4, Name: "Dave", Age: 40, Price: 40, Active: false,
			Status: "archived", Role: 3, CreatedAt: Epoch.Add(96 * time.Hour),
		},
		{
			ID: 5, Name: "alice", Age: 22, Price: 5, Active: true,
			Status: "pending", Role: 2, CreatedAt: Epoch.Add(120 * time.Hour),
		},
		{
			ID: 6, Name: "Eve", Age: 28, Price: 15.75, Active: false,
			Status: "active", Role: 1, CreatedAt: Epoch.Add(144 * time.Hour),
			DeletedAt: &deletedAt,
		},
		{
			ID: 7, Name: "Zoe", Age: 45, Price: 99.99, Active: true,
			Status: "archived", Role: 3, CreatedAt: Epoch.Add(168 * time.Hour),
		},
	}
}

// Case is one filter expression and the rows it must select.
//
// Want is the expectation for every backend the case runs on. Where a
// backend legitimately disagrees — MariaDB's case-insensitive collation,
// Meilisearch's numeric-only timestamps — Differs records the deviation
// so the divergence is asserted rather than tolerated. Skip records the
// backends the case does not apply to at all, each with the reason.
type Case struct {
	Differs map[Backend][]int64
	Skip    map[Backend]string
	Name    string
	Expr    string
	Want    []int64
}

// WantFor returns the expected row IDs for a backend.
func (c Case) WantFor(b Backend) []int64 {
	if want, ok := c.Differs[b]; ok {
		return want
	}
	return c.Want
}

// AppliesTo reports whether the case runs on a backend, and why not.
func (c Case) AppliesTo(b Backend) (string, bool) {
	reason, skipped := c.Skip[b]
	return reason, !skipped
}

// noNullConcept is the shared reason for skipping the null-related cases
// on the two backends whose documents simply omit an absent field rather
// than storing a null for it.
const (
	noNullConcept = "no null concept: an absent attribute is not indexed"
	noStringOrder = "string fields are not ordered by this backend"
)

// Cases returns the shared success corpus: every operation the
// translators support, against the dataset above.
//
// The length is deliberate: a corpus is a data table, and splitting it
// would hide the matrix it exists to show.
func Cases() []Case {
	return []Case{
		// -- Equality and inequality -------------------------------
		{
			Name: "equal string",
			Expr: `name == "Alice"`,
			Want: []int64{1},
			Differs: map[Backend][]int64{
				MariaDB:    {1, 5}, // utf8mb4_*_ci folds case
				Meili:      {1, 5}, // filter equality is case-insensitive
				RediSearch: {1, 5}, // a TEXT field is tokenized case-folded
			},
		},
		{
			Name: "not equal string",
			Expr: `name != "Alice"`,
			Want: []int64{2, 3, 4, 5, 6, 7},
			Differs: map[Backend][]int64{
				MariaDB:    {2, 3, 4, 6, 7},
				Meili:      {2, 3, 4, 6, 7},
				RediSearch: {2, 3, 4, 6, 7},
			},
		},
		{
			Name: "equal int",
			Expr: `age == 30`,
			Want: []int64{1},
		},
		{
			Name: "not equal int",
			Expr: `age != 30`,
			Want: []int64{2, 3, 4, 5, 6, 7},
		},
		{
			Name: "equal float",
			Expr: `price == 30.25`,
			Want: []int64{3},
		},
		{
			Name: "equal bool",
			Expr: `active == true`,
			Want: []int64{1, 3, 5, 7},
		},
		{
			Name: "equal bool false",
			Expr: `active == false`,
			Want: []int64{2, 4, 6},
		},

		// -- Ordering ----------------------------------------------
		{
			Name: "greater than",
			Expr: `age > 30`,
			Want: []int64{3, 4, 7},
		},
		{
			Name: "greater or equal",
			Expr: `age >= 30`,
			Want: []int64{1, 3, 4, 7},
		},
		{
			Name: "less than",
			Expr: `age < 30`,
			Want: []int64{2, 5, 6},
		},
		{
			Name: "less or equal",
			Expr: `age <= 30`,
			Want: []int64{1, 2, 5, 6},
		},
		{
			Name: "float ordering",
			Expr: `price > 20`,
			Want: []int64{3, 4, 7},
		},
		{
			// String ordering is the one comparison whose result is not
			// a property of the translator but of the server's
			// collation: ClickHouse orders bytes, so lowercase 'alice'
			// (0x61) sorts above 'M', while the collations MariaDB and
			// PostgreSQL default to fold case and put it below. Both
			// answers are correct; the case exists to pin which backend
			// gives which.
			Name: "string ordering follows the backend collation",
			Expr: `name > "M"`,
			Want: []int64{5, 7},
			Differs: map[Backend][]int64{
				MariaDB:  {7},
				Postgres: {7},
			},
			Skip: map[Backend]string{
				Meili:      noStringOrder,
				RediSearch: noStringOrder,
			},
		},

		// -- Logical ------------------------------------------------
		{
			Name: "and",
			Expr: `active == true && age > 25`,
			Want: []int64{1, 3, 7},
		},
		{
			Name: "or",
			Expr: `age < 25 || age > 40`,
			Want: []int64{5, 7},
		},
		{
			Name: "not",
			Expr: `!(age > 30)`,
			Want: []int64{1, 2, 5, 6},
			Skip: map[Backend]string{
				RediSearch: "negated numeric ranges are not expressible in the query syntax",
			},
		},
		{
			Name: "bare identifier is a boolean test",
			Expr: `active`,
			Want: []int64{1, 3, 5, 7},
		},
		{
			Name: "negated bare identifier",
			Expr: `!active`,
			Want: []int64{2, 4, 6},
		},
		{
			Name: "nested groups",
			Expr: `(status == "active" || status == "pending") && age >= 28`,
			Want: []int64{1, 3, 6},
		},

		// -- Membership ---------------------------------------------
		{
			Name: "in strings",
			Expr: `status in ["active", "archived"]`,
			Want: []int64{1, 3, 4, 6, 7},
		},
		{
			Name: "in ints",
			Expr: `role in [2, 3]`,
			Want: []int64{2, 4, 5, 7},
		},
		{
			Name: "in single element",
			Expr: `status in ["pending"]`,
			Want: []int64{2, 5},
		},
		{
			Name: "in empty list matches nothing",
			Expr: `status in []`,
			Want: nil,
			Skip: map[Backend]string{
				Meili:      "an empty IN list is a syntax error in the filter grammar",
				RediSearch: "an empty TAG set is a syntax error in the query syntax",
			},
		},

		// -- Null and existence -------------------------------------
		{
			Name: "equal null",
			Expr: `deletedAt == null`,
			Want: []int64{1, 2, 3, 4, 5, 7},
			Skip: map[Backend]string{RediSearch: noNullConcept},
		},
		{
			Name: "not equal null",
			Expr: `deletedAt != null`,
			Want: []int64{6},
			Skip: map[Backend]string{RediSearch: noNullConcept},
		},
		{
			Name: "has",
			Expr: `has(row.deletedAt)`,
			Want: []int64{6},
			Skip: map[Backend]string{RediSearch: noNullConcept},
		},

		// -- String predicates --------------------------------------
		{
			Name: "contains",
			Expr: `name.contains("li")`,
			Want: []int64{1, 3, 5},
		},
		{
			Name: "contains is case sensitive",
			Expr: `name.contains("Al")`,
			Want: []int64{1},
			Differs: map[Backend][]int64{
				MariaDB:    {1, 5}, // _ci collation
				Meili:      {1, 5}, // CONTAINS is case-insensitive
				RediSearch: {1, 5}, // a TEXT field is tokenized case-folded
			},
		},
		{
			// Two characters, not one: RediSearch's MINPREFIX defaults to
			// 2 and drops a shorter prefix term, which would look like a
			// translator bug rather than the server setting it is.
			Name: "startsWith",
			Expr: `name.startsWith("Al")`,
			Want: []int64{1},
			Differs: map[Backend][]int64{
				MariaDB:    {1, 5},
				Meili:      {1, 5},
				RediSearch: {1, 5},
			},
		},
		{
			Name: "endsWith",
			Expr: `name.endsWith("e")`,
			Want: []int64{1, 3, 4, 5, 6, 7},
			Skip: map[Backend]string{
				Meili:      "no suffix predicate in the filter grammar",
				RediSearch: "no suffix predicate in the query syntax",
			},
		},
		{
			Name: "matches",
			Expr: `name.matches("^A.*e$")`,
			Want: []int64{1},
			Differs: map[Backend][]int64{
				MariaDB: {1, 5}, // REGEXP honors the column collation
			},
			Skip: map[Backend]string{
				Meili:      "no regex predicate in the filter grammar",
				RediSearch: "no regex predicate in the query syntax",
			},
		},

		// -- size() --------------------------------------------------
		{
			Name: "size of a string",
			Expr: `name.size() > 3`,
			Want: []int64{1, 3, 4, 5},
			Skip: map[Backend]string{
				Mongo:      "$size applies to arrays, not to string length",
				Meili:      "no length function in the filter grammar",
				RediSearch: "no length function in the query syntax",
			},
		},

		// -- Timestamps ----------------------------------------------
		{
			Name: "timestamp ordering",
			Expr: `createdAt > timestamp("2024-01-04T00:00:00Z")`,
			Want: []int64{4, 5, 6, 7},
			Skip: map[Backend]string{
				RediSearch: "timestamps are indexed as epoch NUMERIC; covered by the numeric cases",
			},
		},
	}
}

// Rejection is one filter expression that must NOT translate, together
// with the sentinel error it must produce.
//
// The rejection matrix is the other half of the corpus: a translator that
// silently accepted `substring()` or a field outside its allow-list would
// pass every success case above and still be broken.
type Rejection struct {
	Differs map[Backend]error
	Skip    map[Backend]string
	Name    string
	Expr    string
	Options []filter.TranslatorOption
	WantErr error
}

// WantFor returns the sentinel a backend must produce.
func (r Rejection) WantFor(b Backend) error {
	if err, ok := r.Differs[b]; ok {
		return err
	}
	return r.WantErr
}

// AppliesTo reports whether the rejection runs on a backend, and why not.
func (r Rejection) AppliesTo(b Backend) (string, bool) {
	reason, skipped := r.Skip[b]
	return reason, !skipped
}

// sqlOnly marks a rejection that only the SQL translators can make.
func sqlOnly() map[Backend]string {
	const reason = "specific to the SQL translators"
	return map[Backend]string{Mongo: reason, Meili: reason, RediSearch: reason}
}

// Rejections returns the shared rejection matrix: every guard the
// translators are supposed to enforce, and the sentinel each must raise.
//
// The length is deliberate: a corpus is a data table, and splitting it
// would hide the matrix it exists to show.
func Rejections() []Rejection {
	return []Rejection{
		// -- Operations no translator implements --------------------
		{
			Name:    "substring is not translatable",
			Expr:    `name.substring(0, 3) == "Ali"`,
			WantErr: filter.ErrUnsupportedOperation,
		},

		// -- Operations only some backends implement -----------------
		{
			Name:    "size outside a comparison",
			Expr:    `name.size()`,
			WantErr: filter.ErrUnsupportedOperation,
			Skip: map[Backend]string{
				// The MongoDB translator renders a bare size() as a
				// $size query rather than refusing it.
				Mongo: "bare size() is a valid $size query",
			},
		},
		{
			Name:    "endsWith is not expressible everywhere",
			Expr:    `name.endsWith("e")`,
			WantErr: filter.ErrUnsupportedOperation,
			Skip: map[Backend]string{
				ClickHouse: "supported",
				MariaDB:    "supported",
				Postgres:   "supported",
				Mongo:      "supported",
			},
		},
		{
			Name:    "matches is not expressible everywhere",
			Expr:    `name.matches("^A")`,
			WantErr: filter.ErrUnsupportedOperation,
			Skip: map[Backend]string{
				ClickHouse: "supported",
				MariaDB:    "supported",
				Postgres:   "supported",
				Mongo:      "supported",
			},
		},
		{
			Name:    "ordering against null is meaningless",
			Expr:    `deletedAt > null`,
			WantErr: filter.ErrUnsupportedOperation,
			Skip: map[Backend]string{
				// The MongoDB translator emits {$gt: null}, which the
				// server accepts and evaluates to false.
				Mongo:      "{$gt: null} is a valid, if useless, query",
				RediSearch: "null is rendered as a tag value, not as a null",
			},
		},

		// -- Policy guards -------------------------------------------
		{
			Name:    "field outside the allow-list",
			Expr:    `passwordHash > ""`,
			Options: []filter.TranslatorOption{filter.WithAllowedFields("name", "age")},
			WantErr: filter.ErrFieldNotAllowed,
		},
		{
			Name: "literal type does not match the declared field kind",
			Expr: `age == "thirty"`,
			Options: []filter.TranslatorOption{
				filter.WithFieldTypes(map[string]filter.FieldKind{"age": filter.FieldKindInt}),
			},
			WantErr: filter.ErrFieldTypeMismatch,
		},
		{
			Name: "list element type does not match the declared field kind",
			Expr: `age in [1, "two"]`,
			Options: []filter.TranslatorOption{
				filter.WithFieldTypes(map[string]filter.FieldKind{"age": filter.FieldKindInt}),
			},
			WantErr: filter.ErrFieldTypeMismatch,
		},
		{
			Name:    "enum value outside the declared set",
			Expr:    `role == 9`,
			Options: []filter.TranslatorOption{filter.WithEnumValues(map[string][]int64{"role": {1, 2, 3}})},
			WantErr: filter.ErrEnumValueNotAllowed,
		},
		{
			Name:    "enum value outside the declared set inside a list",
			Expr:    `role in [1, 9]`,
			Options: []filter.TranslatorOption{filter.WithEnumValues(map[string][]int64{"role": {1, 2, 3}})},
			WantErr: filter.ErrEnumValueNotAllowed,
		},

		// -- Resource guards ------------------------------------------
		{
			Name:    "nesting deeper than the configured limit",
			Expr:    `a == 1 && b == 2 && c == 3 && d == 4 && e == 5`,
			Options: []filter.TranslatorOption{filter.WithMaxDepth(2)},
			WantErr: filter.ErrMaxDepthExceeded,
		},
		{
			Name:    "regex longer than the configured limit",
			Expr:    `name.matches("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")`,
			Options: []filter.TranslatorOption{filter.WithMaxRegexLength(8)},
			WantErr: filter.ErrInvalidRegex,
			Differs: map[Backend]error{
				// These two reject matches() before the pattern is ever
				// measured, so the guard they hit is a different one.
				Meili:      filter.ErrUnsupportedOperation,
				RediSearch: filter.ErrUnsupportedOperation,
			},
		},

		// -- Column-position guards (SQL only) -------------------------
		{
			Name:    "mapped name is an expression",
			Expr:    `name == "x"`,
			Options: []filter.TranslatorOption{filter.WithFieldMapping(map[string]string{"name": "lower(name)"})},
			WantErr: filter.ErrInvalidExpression,
			Skip:    sqlOnly(),
		},
		{
			Name:    "mapped name carries a quote",
			Expr:    `name == "x"`,
			Options: []filter.TranslatorOption{filter.WithFieldMapping(map[string]string{"name": `n" OR 1 = 1 -- `})},
			WantErr: filter.ErrInvalidExpression,
			Skip:    sqlOnly(),
		},
		{
			Name:    "mapped name carries a backtick",
			Expr:    `name == "x"`,
			Options: []filter.TranslatorOption{filter.WithFieldMapping(map[string]string{"name": "n` OR 1 = 1 -- "})},
			WantErr: filter.ErrInvalidExpression,
			Skip:    sqlOnly(),
		},
		{
			Name:    "mapped name is empty",
			Expr:    `name == "x"`,
			Options: []filter.TranslatorOption{filter.WithFieldMapping(map[string]string{"name": ""})},
			WantErr: filter.ErrInvalidExpression,
			Skip:    sqlOnly(),
		},
	}
}
