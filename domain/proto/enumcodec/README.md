# enumcodec

```go
import "github.com/altessa-s/go-atlas/domain/proto/enumcodec"
```

Package `enumcodec` maps a generated proto enum to a domain enum and back from one static table. An undefined proto value is reported as a
`core/errors.FieldError` on its API field path, which the gRPC errstatus canonical rules turn into a BadRequest field violation.

## Key types

| Symbol                                  | Description                                                                                   |
|-----------------------------------------|-----------------------------------------------------------------------------------------------|
| `Codec[P ~int32, D comparable]`         | Immutable bidirectional mapping; safe for concurrent use; the zero `Codec` maps nothing       |
| `New(m map[P]D) Codec[P, D]`            | Copies a one-to-one table of defined, non-zero proto values to non-zero domain values         |
| `Codec.Domain(field, p) (D, error)`     | `UNSPECIFIED` (0) → zero `D`, no error; undefined → `FieldError{field, "unknown value <p>"}`   |
| `Codec.Proto(d) P`                      | Defined `d` → its proto value; zero or unknown `d` → `UNSPECIFIED`                             |

`New` panics on a programming error in the table: an entry for the proto zero value (it always decodes to the zero `D`), a value mapped to the zero
`D` (indistinguishable from unset), or two proto values mapped to one domain value (the reverse direction would be ambiguous). Required-ness is
left to domain validation, so a partial update can leave an enum unset.

## Usage

```go
var gradeCodec = enumcodec.New(map[refv1.Grade]entities.Grade{
    refv1.Grade_GRADE_JUNIOR: entities.GradeJunior,
    refv1.Grade_GRADE_SENIOR: entities.GradeSenior,
})

grade, err := gradeCodec.Domain("referenceItem.roleGrade.grade", p.GetGrade())
if err != nil {
    return nil, err // "referenceItem.roleGrade.grade: unknown value 9"
}
p.Grade = gradeCodec.Proto(item.Grade)
```
