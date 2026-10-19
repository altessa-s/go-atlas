# mongo

```go
import "github.com/altessa-s/go-atlas/data/filter/translators/mongo"
```

Package `mongo` translates filter AST nodes into MongoDB `bson.M` query documents. Supports all comparison, logical, membership, and string operations.

A bare identifier used as a condition becomes a boolean field test — `active` translates to `{active: true}`, `!active` to `{active: {$ne: true}}` — at
the root of an expression and on either side of `$and` / `$or` alike.

## size()

`size()` measures an array by its element count and a string by its code points (`$strLenCP`, as CEL defines it):

| Expression       | Array (or absent/null)                                    | String                     |
|------------------|-----------------------------------------------------------|----------------------------|
| `f.size() == n`  | `{f: {$size: n}}` — an absent or null field never matches | code-point length `== n`   |
| `f.size() != n`  | `{f: {$not: {$size: n}}}` — absent and null match         | code-point length `!= n`   |
| `f.size() > n` … | `$size` of `$ifNull(f, [])` — absent and null count as 0  | code-point length compared |

The string test is guarded by `$type`, so `$strLenCP` never sees another type; a value that is neither an array nor a string has no size and matches no
ordering comparison.

## Fields omitted when zero

A document encoded with `omitempty` lacks a field whose value is zero, and a plain MongoDB query tells the two apart: `{description: ""}` misses a
document without `description`, while `{description: {$ne: ""}}` selects it. Declare such fields with `filter.WithZeroWhenAbsent` — a kind per CEL field
name — and every predicate over them matches a document lacking the field exactly when it matches one storing the zero value:

| Expression          | Translation                                                          |
|---------------------|----------------------------------------------------------------------|
| `description == ""` | `{$or: [{description: ""}, {description: {$exists: false}}]}`        |
| `description != ""` | `{$and: [{description: {$ne: ""}}, {description: {$exists: true}}]}` |
| `retries < 3`       | `{$or: [{retries: {$lt: 3}}, {retries: {$exists: false}}]}`          |

The decision follows MongoDB's own semantics on the stored zero — comparisons match only within a type class, so `description == 0` does not select an
absent description and `description != 0` does; timestamps compare at the millisecond precision of a BSON date; `null` never equals a zero. It covers
comparisons, `in`, `size()`, the string predicates and `matches()`; `has()` on a declared field always holds. Each predicate is exact on its own, so
negation and `&&` / `||` nesting compose. Every `size()` comparison must compare with a numeric literal — anything else is rejected with
`filter.ErrInvalidExpression`. String comparisons are judged byte-wise, as under MongoDB's default simple collation; a query or collection with a locale
collation (which may, for example, ignore whitespace or case) can match a stored `""` where the translator's decision for an absent field differs, so
use the option with the simple collation.

## Decimal fields

A field declared `filter.FieldKindDecimal` (money stored as `Decimal128`) receives its literals as BSON `Decimal128` values read by
`filter.DecimalOperand` from their source spelling, so the server compares them exactly: `price >= 0.1` becomes `{price: {$gte: NumberDecimal("0.1")}}`,
not a comparison with the binary double nearest 0.1, and `1.00000000000000001` stays exact. Values are normalized (`"12.30"` is sent as 12.3, equal for
MongoDB). Lists of `in` are converted element by element, `null` is left alone, and a literal that is not a Decimal128-sized decimal (more than 34
significant digits, an exponent out of range, a non-numeric string) is `filter.ErrFieldTypeMismatch`. Declared with `WithZeroWhenAbsent`, an absent
decimal stands for 0.

## Anchors

MongoDB evaluates `$regex` with PCRE, where `$` also matches just before a final newline. `endsWith(s)` therefore anchors with `\z`, the absolute end,
so `"abc\n"` does not end with `"abc"`; `startsWith(s)` keeps `^`, which without the `m` option matches only at the start. A `matches()` pattern is RE2,
as CEL defines it: every `$` RE2 reads as end of text is rewritten to `\z` before the pattern is sent (`^A.*e$` becomes `^A.*e\z`), while a `$` under
`(?m)`, an escaped `\$` and a `$` in a character class are left alone.

A BSON regex is a C string, so a raw NUL byte in a `contains()` / `startsWith()` / `endsWith()` argument or a `matches()` pattern is sent as `\x00`,
which matches the same byte; inside `\Q…\E` the quote is closed around it (`\E\x00\Q`), so the pattern keeps its meaning.

## Security

`matches()` passes the user-supplied pattern through to MongoDB's `$regex`. Host-side validation compiles it with Go's RE2 engine (which cannot
backtrack) and enforces a length cap, but MongoDB executes the pattern with its own PCRE-family engine, where a crafted pattern can backtrack
catastrophically — a DB-side ReDoS the length cap bounds but does not eliminate. Expose `matches()` only to trusted callers, or tighten the cap via
`filter.WithMaxRegexLength`. `contains()`, `startsWith()`, and `endsWith()` escape their argument with `regexp.QuoteMeta` and are not affected.

## Concurrency

A `Translator` keeps per-call state (nesting depth) and is **not** safe for concurrent use. Construct one per goroutine, or guard it with a mutex.

## Literal operands

Comparison values, `in` list elements, and string-function arguments must be literals. Dynamic operands such as `a == b`, `a in [b]`, or `a.contains(b)`
return `filter.ErrUnsupportedOperation`; a field reference is never converted into a string value.
