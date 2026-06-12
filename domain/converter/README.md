# converter

```go
import "github.com/altessa-s/go-atlas/domain/converter"
```

Package `converter` provides high-performance struct-to-struct conversion with field mapping, filtering,
embedded structs, slices, maps, and protocol buffers. It caches type metadata and pools allocations for
optimal throughput.

## Options

| Option                       | Description                                       |
|------------------------------|---------------------------------------------------|
| `WithCodecs`                 | Register custom codec pipeline                    |
| `WithIgnoreFields`           | Skip specific fields by name (case-insensitive)   |
| `WithFieldMappings`          | Map source field names to destination names        |
| `WithIgnoreZeroValues`       | Skip zero-valued source fields                    |
| `WithIgnoreNilValues`        | Skip nil source fields (pointers, slices, maps)   |
| `WithSparseMerge`            | Sparse merge (partial update): skip nil sources, overwrite with non-nil pointers, clear on present-but-empty nested struct pointers |
| `WithHandleEmbeddedStructs`  | Recursively expand embedded (anonymous) structs   |
| `WithOverflowCheck`          | Panic on narrowing numeric overflow               |

## Value-handling semantics

Three mutually exclusive options control how source values are applied to the destination. The last one passed wins. All examples below
share these types:

```go
type Address struct {
	City *string
	Zip  *string
}

type User struct {
	Name    *string
	Age     *int
	Address *Address
	Tags    []string
}

dst := User{
	Name: ptr("Ann"), Age: ptr(30),
	Address: &Address{City: ptr("Berlin"), Zip: ptr("10115")},
	Tags:    []string{"vip"},
}
src := User{Name: ptr("Bob"), Address: &Address{City: ptr("Paris")}}
```

### Default (no option)

Every matching exported field is copied. Nil struct-field pointers are skipped, but everything else overwrites the destination: a nil
slice or map is assigned as-is and wipes the destination field, a zero scalar overwrites the stored value, and a nested struct of the
same type is assigned as a whole pointer (the destination starts aliasing the source).

```go
converter.Convert(&src, &dst)
// dst.Name        == "Bob"
// dst.Age         == 30      (nil pointer skipped)
// dst.Address.Zip == nil     (whole Address pointer replaced — Zip lost)
// dst.Tags        == nil     (nil slice assigned — wiped)
```

### WithIgnoreZeroValues

Skips every zero source value: zero scalars, nil pointers, nil slices and maps, zero structs. Useful for "copy only what is set", but an
explicit empty value can never be written — there is no way to clear a destination field.

```go
converter.Convert(&src, &dst, converter.WithIgnoreZeroValues())
// dst.Tags == []string{"vip"} (nil slice skipped)
// src.Name = ptr("") would still be copied: a non-nil pointer is not zero.
// src.Age int = 0 (non-pointer) would be skipped — even if 0 was meant.
```

### WithIgnoreNilValues

Skips only nil pointers, slices, and maps; a pointer to a zero value is written, so explicit clearing of scalars works. Nested structs
of the same type are still assigned wholesale, so untouched sibling fields inside them are lost.

```go
converter.Convert(&src, &dst, converter.WithIgnoreNilValues())
// dst.Tags        == []string{"vip"} (nil slice skipped)
// dst.Address.Zip == nil             (whole Address pointer replaced — Zip lost)
```

### WithSparseMerge

Treats the source as a sparse patch applied onto the existing destination — the Go-struct analogue of JSON Merge Patch (RFC 7386). The
option is mutually exclusive with `WithIgnoreZeroValues` and subsumes `WithIgnoreNilValues` — nil sources are skipped the same way. For
each source field, at every nesting depth:

| Source field value                                  | Effect on destination            |
|-----------------------------------------------------|----------------------------------|
| nil pointer / slice / map                           | skipped, destination unchanged   |
| non-nil scalar pointer (incl. pointer to zero)      | written — explicit empty clears  |
| non-nil struct pointer with at least one set field  | merged recursively               |
| non-nil struct pointer with no set fields           | destination field zeroed         |
| non-nil slice                                       | replaces the destination slice   |

```go
converter.Convert(&src, &dst, converter.WithSparseMerge())
// dst.Name         == "Bob"
// dst.Age          == 30              (nil pointer skipped)
// dst.Address.City == "Paris"
// dst.Address.Zip  == "10115"         (untouched sibling preserved)
// dst.Tags         == []string{"vip"} (nil slice skipped)

// Clear a scalar with an explicit empty value:
converter.Convert(&User{Name: ptr("")}, &dst, converter.WithSparseMerge())
// dst.Name == ""

// Remove a nested object entirely with a present-but-empty struct:
converter.Convert(&User{Address: &Address{}}, &dst, converter.WithSparseMerge())
// dst.Address == nil
```

The source must express optionality through pointers: a non-pointer scalar is always "present" and overwrites the destination,
including with its zero value. A non-pointer nested struct carries no presence signal — it is always merged field-by-field and never
triggers the clear-on-empty rule. Structs without exported fields (`time.Time` and similar opaque types) cannot be merged
field-by-field and are assigned wholesale.

## Functions

| Function        | Description                                            |
|-----------------|--------------------------------------------------------|
| `Convert`       | One-shot struct/slice conversion                       |
| `New`           | Create reusable generic `Converter[T, U]`              |
| `NewAny`        | Create reusable non-generic `Converter[any, any]`      |
| `ConvertSeq`    | Lazy iterator over converted slice elements            |
| `ConvertMapSeq` | Lazy iterator over converted map key-value pairs       |
| `IndirectType`  | Dereference all pointer layers from a `reflect.Type`   |

## Subpackages

| Package                                          | Description                                          |
|--------------------------------------------------|------------------------------------------------------|
| [codec](./codec)                                 | Pluggable codec interface and chain execution engine |
| [codecs/optionalcodec](./codecs/optionalcodec)   | `core/types/optional.Optional[T]` field codec        |
