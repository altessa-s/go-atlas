# bsoncodec

```go
import "github.com/altessa-s/go-atlas/store/mongo/bsoncodec"
```

MongoDB registry for [`optional.Optional[T]`](https://github.com/altessa-s/go-atlas-core/tree/develop/types/optional), `shopspring/decimal` and
`civil.Date`, keeping database dependencies out of universal value types. Existing driver codecs and custom inner-type codecs continue to work.

Use `decimal128.Fits` and `decimal128.Canonical` from `github.com/altessa-s/go-aip/decimal128` to validate and normalize decimals.

## API

| Function | Behavior |
|----------|----------|
| `NewRegistry()` | Driver defaults plus codecs for every Optional instantiation, `shopspring/decimal`, `civil.Date` and [`go-atlas-core/time/civil.Date`](https://github.com/altessa-s/go-atlas-core/tree/develop/time/civil); configure before concurrent use |
| `RegisterDateType[T](reg, opts...)` | Adds the calendar-date codec for a struct with `Year int`, `Month time.Month`, `Day int` fields |
| `decimal128.Fits(d)` | Reports whether the decimal codec stores `d` exactly — the encoder's own verdict, for validating input before it reaches the store |
| `decimal128.Canonical(d)` | `d` as the codec stores and reads it back (≤ 34 digits, exponent in [-6176, 6111], zero as `decimal.Zero`), `ok` false when it does not fit; keep this form, not `d`, when comparing or printing — `decimal128.Fits` accepts a zero with any exponent |

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

The [`store/mongo`](../README.md) client constructor installs this registry unless explicitly overridden. Injected clients and custom registries remain
the caller's responsibility. RedactedString's fixed redaction hook also works with this registry and with the driver's default registry.

Pass the same registry to the [`mapping/behavior/translators/mongo`](../../../mapping/behavior/translators/mongo) translators with `WithRegistry`, so that
they write decimals, calendar dates and Optionals as units instead of folding them field by field.

## Calendar dates

A calendar date without time or zone is stored as its `YYYY-MM-DD` string: nothing can shift it across a day boundary, and strings of that form sort in
date order, so range queries and indexes work on them. `NewRegistry` registers the codec for `civil.Date` (`cloud.google.com/go/civil`) and for go-atlas
[`go-atlas-core/time/civil.Date`](https://github.com/altessa-s/go-atlas-core/tree/develop/time/civil) (zero stored as `""`, matching its text form);
`RegisterDateType[T](reg, opts...)` registers it for any struct with the fields `Year int`, `Month time.Month` and `Day int` — a service's own date type
— and fails with `ErrNotADateType` for any other.

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
// store/mongo: pass mongoOptions.Client().SetRegistry(reg) through WithClientOptions; it overrides the default registry.
```

## Decimal

`decimal.Decimal`, `*decimal.Decimal`, `decimal.NullDecimal` and `*decimal.NullDecimal` use the strict decimal codecs. Valid values are stored as BSON
`Decimal128`, exactly: trailing zeros fold into the exponent, and a value that still needs more than 34 significant digits, or an exponent outside the
Decimal128 range, fails with `ErrDecimalOutOfRange` instead of being rounded or panicking. `decimal128.Fits(d)` reports
whether it succeeds, so a validator rejects exactly the values the encoder would. Nil decimal pointers encode as BSON null and decode from null as nil.

| Stored BSON | Decoded as |
|-------------|------------|
| `Decimal128` | The exact value; NaN or ±Infinity fail with `ErrDecimalNotFinite` |
| `Int32`, `Int64` | The exact integer |
| `Double` | Error: binary floating point is never converted silently |
| Null | Zero `Decimal`; invalid `NullDecimal`; nil decimal pointer |
