# googletype

```go
import "github.com/altessa-s/go-atlas/domain/proto/googletype"
```

Package `googletype` converts between the `google.type` / `google.protobuf` well-known types an API carries and the Go values a domain uses. The
converters do conversion only: a decoder rejects input the wire type cannot represent with a sentinel error that carries no API field path. Domain
policy (a non-negative amount, an expected currency) and the mapping onto request field paths belong to the caller. Zero values are absent (nil) on
output.

## Key types

| Function                                      | Description                                                                                   |
|-----------------------------------------------|-----------------------------------------------------------------------------------------------|
| `MoneyFromProto(p)`                           | `google.type.Money` → `money.Money`, exactly units + nanos·10⁻⁹; nil is the zero Money        |
| `MoneyToProto(m)`                             | `money.Money` → `google.type.Money`, exactly; the zero Money (no currency) is nil             |
| `DateFromProto(d)`                            | `google.type.Date` → `civil.Date`; nil or all-zero is the zero date; else a full date, 1–9999 |
| `DateToProto(d)`                              | `civil.Date` → `google.type.Date`; the zero date is nil                                       |
| `WeekdayFromProto(d)`                         | `google.type.DayOfWeek` (MONDAY=1 … SUNDAY=7) → `time.Weekday` (Sunday=0)                     |
| `WeekdaysFromProto(days)`                     | Repeated form; an offending element is an `*IndexError`; an empty list is nil                 |
| `WeekdayToProto(w)` / `WeekdaysToProto(days)` | `time.Weekday` → `google.type.DayOfWeek`; out of range → `DAY_OF_WEEK_UNSPECIFIED`            |
| `DurationFromProto(d)`                        | `google.protobuf.Duration` → `time.Duration`; nil is zero; saturates like `AsDuration`        |
| `DurationToProto(d)`                          | Zero is nil                                                                                   |
| `TimestampToProto(t)`                         | The zero `time.Time` is nil                                                                   |
| `IndexError`                                  | `{Index int; Err error}` with `Unwrap`: the element a list decoder rejected                   |

## Errors

| Error                | Returned by         | Input                                                                       |
|----------------------|---------------------|-----------------------------------------------------------------------------|
| `ErrImpossibleDate`  | `DateFromProto`     | Impossible or partial date (Feb 30, zero month or day, year 0 or > 9999)    |
| `ErrUnknownWeekday`  | `WeekdayFromProto`  | `DAY_OF_WEEK_UNSPECIFIED` or an undefined value                             |
| `ErrInvalidDuration` | `DurationFromProto` | Duration failing `durationpb.CheckValid`                                    |
| `ErrInvalidNanos`    | `MoneyFromProto`    | `nanos` outside ±999,999,999                                                |
| `ErrSignMismatch`    | `MoneyFromProto`    | `units` and `nanos` of opposite signs                                       |
| `ErrMissingCurrency` | `MoneyFromProto`    | Empty `currency_code`                                                       |
| `ErrUnknownCurrency` | `MoneyFromProto`    | `currency_code` that is not ISO 4217 (matched case-insensitively)           |
| `ErrPrecision`       | `MoneyToProto`      | More than nine significant fractional digits — round first, never truncated |
| `ErrOverflow`        | `MoneyToProto`      | Integer part outside int64                                                  |

## Usage

```go
budget, err := googletype.MoneyFromProto(req.GetProvider().GetMonthlyBudgetLimit())
switch {
case errors.Is(err, googletype.ErrInvalidNanos):
    return nil, coreerrs.NewFieldError("provider.monthlyBudgetLimit.nanos", "must be within ±999999999")
case err != nil:
    return nil, coreerrs.NewFieldError("provider.monthlyBudgetLimit", err.Error())
case budget.IsNegative():
    return nil, coreerrs.NewFieldError("provider.monthlyBudgetLimit", "must not be negative")
}
resp.CrawlDelay = googletype.DurationToProto(delay)
```
