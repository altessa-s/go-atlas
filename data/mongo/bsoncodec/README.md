# bsoncodec

```go
import "github.com/altessa-s/go-atlas/data/mongo/bsoncodec"
```

MongoDB registry for [`optional.Optional[T]`](../../../core/types/optional), `shopspring/decimal` and `civil.Date`, keeping database
dependencies out of universal value types. Existing driver codecs and custom inner-type codecs continue to work.

## API

| Function | Behavior |
|----------|----------|
| `NewRegistry()` | Driver defaults plus codecs for every Optional instantiation, `shopspring/decimal` and `civil.Date`; configure before concurrent use |

## Usage

```go
reg := bsoncodec.NewRegistry()
client, err := mongo.Connect(options.Client().SetRegistry(reg))
```

For standalone BSON, set the same registry on both `bson.NewEncoder(bson.NewDocumentWriter(writer))` and
`bson.NewDecoder(bson.NewDocumentReader(reader))` with `SetRegistry(reg)`. `bson.Marshal` and `bson.Unmarshal` use the driver's default registry and
cannot serialize Optional without configuration; they return `optional.ErrBSONCodecRequired` for present Optional fields. None with `omitempty` is
omitted without invoking the codec.

## Semantics

| Value | BSON |
|-------|------|
| `Some(v)` | Encoding of `v`, using this registry |
| `None` | Null; omitted when the field has `omitempty` |
| Null on decode | None |
| Missing field | Existing destination field unchanged; None on a fresh destination |
| Invalid inner value | Error; the Optional destination remains unchanged |
| `Some(nil)` | Null; decodes as None because BSON has no presence bit |

The [`data/mongo`](../README.md) client constructor installs this registry unless explicitly overridden. Injected clients and custom registries remain
the caller's responsibility. RedactedString's fixed redaction hook also works with this registry and with the driver's default registry.

Pass the same registry to the [`domain/behavior/translators/mongo`](../../../domain/behavior/translators/mongo) translators with `WithRegistry`,
so that they write decimals, civil dates and Optionals as units instead of folding them field by field.

## Civil dates

`civil.Date` (`cloud.google.com/go/civil`), a calendar date without time or zone, is stored as its `YYYY-MM-DD` string: nothing can shift it
across a day boundary, and strings of that form sort in date order, so range queries and indexes work on them. The zero `Date` is stored as
null; an invalid date or a year outside 0–9999 fails with `ErrInvalidDate`. Decoding accepts only such strings and null — a BSON date is an
instant, and reading it as a calendar date would pick a zone silently.

## Decimal

`decimal.Decimal`, `*decimal.Decimal` and `decimal.NullDecimal` are stored as BSON `Decimal128`, exactly: trailing zeros fold into the exponent, and a
value that still needs more than 34 significant digits, or an exponent outside the Decimal128 range, fails with `ErrDecimalOutOfRange` instead of
being rounded or panicking.

| Stored BSON | Decoded as |
|-------------|------------|
| `Decimal128` | The exact value; NaN or ±Infinity fail with `ErrDecimalNotFinite` |
| `Int32`, `Int64` | The exact integer |
| `Double` | Error: binary floating point is never converted silently |
| Null | Zero `Decimal`; invalid `NullDecimal`; nil pointer |
