# moneypb

```go
import "github.com/altessa-s/go-atlas/domain/money/moneypb"
```

Conversion between [`money.Money`](..) and `google.type.Money`. The date, duration, timestamp and weekday converters that shared the former
`mapping/googletype` package live in [go-convert `googletype`](https://github.com/altessa-s/go-convert/tree/develop/googletype).

## API

| Function                                  | Behavior                                                                                  |
|-------------------------------------------|-------------------------------------------------------------------------------------------|
| `MoneyFromProto(p)`                       | Exact conversion; sentinel errors for bad nanos, sign mismatch, missing or unknown currency |
| `MoneyToProto(m)`                         | Exact conversion; `ErrPrecision` or `ErrOverflow` when the amount does not fit           |
| `MoneyToProtoFitted(m)`                   | Rounds to nine fractional digits and saturates instead of failing                        |
| `MoneyFromProtoAt(field, p)`              | `MoneyFromProto` with a `FieldError` on the API field path                                |
| `NonNegativeMoneyFromProtoAt(field, p)`   | Same, and rejects negative amounts                                                        |
