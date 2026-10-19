# money

```go
import "github.com/altessa-s/go-atlas/domain/money"
```

Package `money` provides an amount of a currency with exact decimal arithmetic (`shopspring/decimal`), explicit rounding to the currency's minor units
and allocation without losing a minor unit. Store amounts with the [`data/mongo/bsoncodec`](../../data/mongo/bsoncodec) decimal codec, which keeps them
as exact `Decimal128`.

## Key types

| Symbol             | Description                                                                                                        |
|--------------------|--------------------------------------------------------------------------------------------------------------------|
| `Currency`         | Code plus minor units; `ParseCurrency` / `MustParseCurrency` (active ISO 4217 codes), `NewCurrency` (custom units) |
| `Money`            | Immutable amount of a currency: `New`, `Parse`, `FromMinor`, `Zero`                                                |
| `RoundingMode`     | `HalfEven` (banker's), `HalfUp`, `Down`, `Up`, `Floor`, `Ceiling`                                                  |
| `AllocationMethod` | `LargestRemainder` (default of `Allocate` and `Split`), `HalfUpLastRemainder`                                      |
| `ExchangeRate`     | `Value` units of `Quote` per 1 `Base`; `Validate`, `Inverse`, `String`                                             |

## Operations

| Method                    | Behavior                                                                                          |
|---------------------------|---------------------------------------------------------------------------------------------------|
| `Add`, `Sub`, `Cmp`       | Same currency only, `ErrCurrencyMismatch` otherwise                                               |
| `Mul(factor)`             | Exact product, every decimal place kept                                                           |
| `Round(mode)`             | Rounds to the currency's minor units                                                              |
| `MinorUnits()`            | `int64` minor units; `ErrNotRounded` for extra places, `ErrOverflow` beyond `int64`               |
| `Allocate(ratios...)`     | Parts proportional to the ratios, summing to the amount exactly (largest remainder)               |
| `AllocateBy(method, ...)` | `Allocate` with an explicit `AllocationMethod`                                                    |
| `Split(n)`                | `n` parts as equal as possible                                                                    |
| `String()`                | `12.30 EUR`, with exactly the currency's minor units unless the amount has more                   |
| `Convert(rate, mode)`     | Amount in the rate's quote currency, rounded with `mode`; the amount must be in the base currency |

## Rounding

Arithmetic never rounds on its own, so a computation keeps its precision until the caller decides where the cent goes:

```go
price, _ := money.Parse("19.99", "EUR")
vat := price.Mul(decimal.RequireFromString("0.2")).Round(money.HalfEven) // 4.00 EUR (3.998)
```

## Allocation

`Allocate` uses the largest remainder method: every part gets its share rounded toward zero, and the minor units left over go one each to the parts with
the largest discarded remainders, ties to the earlier part. Shares are computed with exact rational arithmetic, so the result does not depend on the
order or scale of the ratios. The amount must be in whole minor units; a negative amount is split as its absolute value and every part negated.

`AllocateBy(HalfUpLastRemainder, ratios...)` follows the convention where every share but the last is rounded half-up and the last takes the remainder.
When the rounded-up shares would push the last one below zero, units are taken back one at a time from the earlier shares rounded up the most, ties to
the later share, so no share is negative. Exact shares are computed on decimals rounded to `HalfUpLastRemainderPrecision` (28) places, matching
implementations of this convention on `shopspring/decimal` share for share.

| Method                | 100.00 split 1:1:1  | 0.03 split six ways                |
|-----------------------|---------------------|------------------------------------|
| `LargestRemainder`    | 33.34, 33.33, 33.33 | 0.01, 0.01, 0.01, 0.00, 0.00, 0.00 |
| `HalfUpLastRemainder` | 33.33, 33.33, 33.34 | 0.01, 0.01, 0.01, 0.00, 0.00, 0.00 |

```go
rent, _ := money.Parse("1000.00", "EUR")
parts, _ := rent.Allocate(decimal.NewFromInt(1), decimal.NewFromInt(1), decimal.NewFromInt(1))
// 333.34 EUR, 333.33 EUR, 333.33 EUR
```

## Exchange rates

An `ExchangeRate` is a plain value: `Value` units of `Quote` per one unit of `Base`. `Validate` requires two distinct, set currencies and a strictly
positive value — a missing rate is never replaced by zero or one.

| Function                                     | Behavior                                                                                   |
|----------------------------------------------|--------------------------------------------------------------------------------------------|
| `NormalizeRate(base, quote, value, nominal)` | Per-unit rate from a quote for `nominal` units: 5500 KZT per 100 RUB ⇒ 55 KZT per 1 RUB    |
| `ExchangeRate.Inverse()`                     | The reciprocal pair: 100 RUB per 1 EUR ⇒ 0.01 EUR per 1 RUB                                |
| `CrossRate(baseQuote, targetQuote)`          | "target per 1 base" from two quotes in a common currency: EUR/RUB, USD/RUB ⇒ USD per 1 EUR |
| `Money.Convert(rate, mode)`                  | `amount × Value` in `Quote`, rounded to its minor units                                    |

Divisions keep `RateDivisionPrecision` (28) fractional digits, rounded half away from zero, so repeating fractions terminate without distorting rates
stored with a dozen digits. Conversion itself never divides: invert or cross the rate first.

```go
rate, _ := money.NormalizeRate(kzt, rub, decimal.RequireFromString("17.85"), 100) // 0.1785 RUB per 1 KZT
price, _ := money.Parse("125000.00", "KZT")
inRUB, _ := price.Convert(rate, money.HalfUp) // 22312.50 RUB
```

## Errors

| Error                 | Meaning                                                            |
|-----------------------|--------------------------------------------------------------------|
| `ErrUnknownCurrency`  | Not an active ISO 4217 code                                        |
| `ErrInvalidCurrency`  | Empty code, minor units out of range, or a zero `Currency`         |
| `ErrCurrencyMismatch` | Amounts of different currencies combined                           |
| `ErrNotRounded`       | The operation needs whole minor units                              |
| `ErrOverflow`         | Minor units beyond `int64` (`MinorUnits`, `Allocate`, `Split`)     |
| `ErrInvalidRatios`    | No ratios, a negative ratio, ratios summing to zero, or `Split(0)` |
| `ErrNonPositiveRate`  | An exchange rate, published value or nominal that is not positive  |
| `ErrPairMismatch`     | Exchange rates that do not form the expected currency pair         |
