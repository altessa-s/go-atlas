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
| `RegisterDateType[T](reg, opts...)` | Adds the calendar-date codec for a struct with `Year int`, `Month time.Month`, `Day int` fields |

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
so that they write decimals, calendar dates and Optionals as units instead of folding them field by field.

## Calendar dates

A calendar date without time or zone is stored as its `YYYY-MM-DD` string: nothing can shift it across a day boundary, and strings of that form
sort in date order, so range queries and indexes work on them. `NewRegistry` registers the codec for `civil.Date` (`cloud.google.com/go/civil`);
`RegisterDateType[T](reg, opts...)` registers it for any struct with the fields `Year int`, `Month time.Month` and `Day int` — a service's own
date type — and fails with `ErrNotADateType` for any other.

| Stored BSON        | Encoded from / decoded to                                                                       |
|--------------------|-------------------------------------------------------------------------------------------------|
| `"YYYY-MM-DD"`     | A valid date in years 0–9999; anything else fails with `ErrInvalidDate`                          |
| Null               | The zero date (the default encoding of it); a nil pointer                                       |
| `""`               | The zero date; written instead of null with `WithDateZeroAsEmptyString()`                        |
| BSON date, others  | Error: an instant read as a calendar date would pick a zone silently                            |

```go
reg := bsoncodec.NewRegistry()
if err := bsoncodec.RegisterDateType[civil.Date](reg, bsoncodec.WithDateZeroAsEmptyString()); err != nil { // the service's own civil.Date
    return err
}
// data/mongo: pass mongoOptions.Client().SetRegistry(reg) through WithClientOptions; it overrides the default registry.
```

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
