# civil

```go
import "github.com/altessa-s/go-atlas/core/time/civil"
```

`Date` is a calendar date without a time zone: due dates, service periods, effective dates, deadlines. Converting it between zones can never move it to
another day. No external dependencies.

| API | Purpose |
|-----|---------|
| `New(y, m, d)` / `MustNew` | Validated constructor (years 1–9999); `ErrInvalidDate` otherwise |
| `Parse(s)` | `"YYYY-MM-DD"`; `""` yields the zero date |
| `Of(t)` / `Today(loc)` | Calendar date of an instant in its location / today in `loc` |
| `IsZero`, `Valid`, `String` | Zero means "no date" and formats as `""` |
| `Time`, `In(loc)`, `Weekday` | Midnight of the date in UTC / in `loc` |
| `Compare`, `Before`, `After`, `AddDays`, `DaysSince` | Ordering and day arithmetic over the whole 1–9999 range |
| `AddMonthsClamped(n, anchorDay)`, `DaysIn` | Month steps that keep an anchor day, clamped in short months (31 Jan → 28/29 Feb → 31 Mar) |

## No date

The zero `Date` means "no date": it sorts before every date, `Time` and `In` return the zero `time.Time`, and `DaysSince` with it is 0. `AddDays` and
`AddMonthsClamped` keep it zero, and also return the zero `Date` for an invalid receiver or a result outside years 1–9999, so arithmetic never invents a
date. An anchor day below 1 gives the 1st. `Weekday` of the zero `Date` is meaningless; check `IsZero` first.

## Encoding

- **Text / JSON**: `"YYYY-MM-DD"`, the zero date as `""`. `UnmarshalJSON` also accepts the legacy object form `{"Year":2026,"Month":10,"Day":9}` written
  by encoding/json for date structs without text methods, so stored documents keep decoding after a service adopts this type. Encoding an invalid
  non-zero date fails with `ErrInvalidDate`.
- **JSON `null`** decodes to the zero date.
- **MongoDB**: `data/mongo/bsoncodec.NewRegistry` registers the codec — the `"YYYY-MM-DD"` string, the zero date as `""`. The codec applies `Valid`, so
  year 0 is refused when writing and reading.

```go
due := civil.MustNew(2026, time.January, 31)
next := due.AddMonthsClamped(1, 31) // 2026-02-28
if civil.Today(loc).After(due) {
	// overdue
}
```
