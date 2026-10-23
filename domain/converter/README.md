# converter

```go
import "github.com/altessa-s/go-atlas/domain/converter"
```

Package `converter` provides high-performance struct-to-struct conversion with field mapping, filtering, embedded structs, slices, maps, and protocol
buffers. It caches type metadata and pools allocations for optimal throughput.

## Options

| Option                       | Description                                       |
|------------------------------|---------------------------------------------------|
| `WithErrorCodecs`            | Register error-returning codecs with full paths    |
| `WithCodecs`                 | Register custom codec pipeline                    |
| `WithIgnoreFields`           | Skip fields by leaf name where the walk descends   |
| `WithIgnorePaths`            | Skip an exact source path (`smtp.password`)        |
| `WithCopySlices`             | Copy assignable slices instead of aliasing         |
| `WithFieldMappings`          | Map source field names to destination names        |
| `WithIgnoreZeroValues`       | Skip zero-valued source fields                    |
| `WithIgnoreNilValues`        | Skip nil source fields (pointers, slices, maps)   |
| `WithSparseMerge`            | Apply a sparse partial update; present-but-empty struct pointers clear the field |
| `WithHandleEmbeddedStructs`  | Recursively expand embedded (anonymous) structs   |
| `WithOverflowCheck`          | Panic on narrowing numeric overflow               |

### Path-aware ignore and copied slices

`WithIgnoreFields("password")` matches a leaf name at every depth the converter
walks; an assignable same-type nested value is copied wholesale and is not
rewritten. Use `WithIgnorePaths("smtp.password")` to ignore exactly one
location; a parent path such as `WithIgnorePaths("smtp")` skips the whole
subtree. Slice, array and map fields contribute one path segment, so
`items.secret` matches every element of `Items`; indices and statically typed map keys are
never part of the path. A promoted source field flattened into the destination
uses its promoted path. A destination-only embedded field adds no segment; the
embedded field name is a path segment only when both sides declare that
embedded field.
Interfaces are traversed only when an ignore path lies below them. Dynamic
structs, pointers, slices, arrays, maps, and scalars are supported; nil interfaces
and typed nils retain their distinction. A nil interface clears an interface
destination but leaves a concrete destination unchanged. A dynamic value that
is not assignable to a non-empty destination interface leaves it unchanged.
Containers are detached before writes,
and a per-traversal cycle guard leaves destination values unchanged at back
edges with ignored descendants. Back edges without ignored descendants retain
the source reference. Dynamic slices use fresh zero elements, like static slice
conversion; dynamic arrays merge into existing destination elements.
String keys of maps reached through interfaces act as path segments: an ignored
key is omitted unless the destination already has it, in which case it is kept.

In plain overwrite mode, nested same-type structs walked for an ignore path
preserve the source's unexported state. The top-level struct passed to `Convert`
keeps destination unexported state; root slice elements still use nested semantics. Ignored exported fields and nil source
pointers keep their destination values. Sparse merge, ignore-zero, and ignore-nil
modes retain field-wise behavior: unexported state is not copied in those modes.
Reference-valued unexported state remains shallow-copied.

`WithCopySlices` gives the destination its own backing array for assignable
slices, recursively through exported struct fields, arrays and map values. A
nil source stays nil and an empty non-nil source stays empty non-nil. Pointers and
interfaces stay aliased because cloning them would change identity, except a
`*[]T` field, whose slice is cloned; conversion paths that build a new slice
already copy.

### Error-returning conversion

`Convert` panics for invalid type combinations, overflow, and malformed input.
`ConvertE` has the same signature but returns those panics as errors, which is
safer for untrusted input and for slices that may contain nil message
pointers. The destination may be partially populated when an error is returned.

`WithErrorCodecs` accepts `convcodec.CodecE` middleware with full source paths.
Error codecs execute before assignable and primitive shortcuts, followed by
legacy codecs and built-in conversion. Error codecs never run for statically
typed map keys and never receive the internal map-key sentinel. A dynamic root
map key that collides with the sentinel is treated as map-key context and is
not exposed to error codecs; legacy codecs and built-in conversion still run. Every error
returned by a `CodecE` is returned unchanged by `ConvertE`, preserving identity,
including `*core/errors.FieldError` and its path. The existing `Codec` and
`Run` APIs retain their behavior. `enumcodec.Codec.ErrorCodec()` supplies a
bidirectional enum adapter that rejects unknown non-zero values.

## Value-handling semantics

Three mutually exclusive options control how source values are applied to the destination. The last one passed wins. All examples below share these
types:

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

Every matching exported field is copied. Nil struct-field pointers are skipped, but everything else overwrites the destination: a nil slice or map is
assigned as-is and wipes the destination field, a zero scalar overwrites the stored value, and a nested struct of the same type is assigned as a whole
pointer (the destination starts aliasing the source).

```go
converter.Convert(&src, &dst)
// dst.Name        == "Bob"
// dst.Age         == 30      (nil pointer skipped)
// dst.Address.Zip == nil     (whole Address pointer replaced — Zip lost)
// dst.Tags        == nil     (nil slice assigned — wiped)
```

### WithIgnoreZeroValues

Skips every zero source value: zero scalars, nil pointers, nil slices and maps, zero structs. Useful for "copy only what is set", but a zero scalar can
never be written — there is no way to zero a destination field with it. Non-nil empty slices and maps and non-nil pointers to zero values are not zero,
so they still overwrite the destination. During dynamic traversal, zero checks
apply at field boundaries and do not reapply to dereferenced pointer pointees.

```go
converter.Convert(&src, &dst, converter.WithIgnoreZeroValues())
// dst.Tags == []string{"vip"} (nil slice skipped)
// src.Name = ptr("") would still be copied: a non-nil pointer is not zero.
// src.Age int = 0 (non-pointer) would be skipped — even if 0 was meant.
```

### WithIgnoreNilValues

Skips only nil pointers, slices, and maps; a pointer to a zero value is written, so explicit clearing of scalars works. Nested structs of the same type
are still assigned wholesale, so untouched sibling fields inside them are lost.

```go
converter.Convert(&src, &dst, converter.WithIgnoreNilValues())
// dst.Tags        == []string{"vip"} (nil slice skipped)
// dst.Address.Zip == nil             (whole Address pointer replaced — Zip lost)
```

### WithSparseMerge

Treats the source as a sparse patch applied onto the existing destination — the Go-struct analogue of JSON Merge Patch (RFC 7386). The option is
mutually exclusive with `WithIgnoreZeroValues` and subsumes `WithIgnoreNilValues` — nil sources are skipped the same way. For each source field, at
every nesting depth:

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

During dynamic traversal, an interface holding a present-but-empty `*Struct`
also clears its destination pointer. The interface retains the concrete pointer
type: it holds `(*Struct)(nil)` and is itself non-nil.

The source must express optionality through pointers: a non-pointer scalar is always "present" and overwrites the destination, including with its zero
value. A non-pointer nested struct carries no presence signal — it is always merged field-by-field and never triggers the clear-on-empty rule. Structs
without exported fields (`time.Time` and similar opaque types) cannot be merged field-by-field and are assigned wholesale.

## Functions

| Function        | Description                                            |
|-----------------|--------------------------------------------------------|
| `Convert`       | One-shot struct/slice conversion                       |
| `New`           | Create reusable generic `Converter[T, U]`              |
| `NewShared`     | Like `New`, but reuses the shared global type caches   |
| `NewAny`        | Create reusable non-generic `Converter[any, any]`      |
| `ConvertSeq`    | Lazy iterator over converted slice elements            |
| `ConvertMapSeq` | Lazy iterator over converted map key-value pairs       |
| `IndirectType`  | Dereference all pointer layers from a `reflect.Type`   |

## Subpackages

| Package                                          | Description                                          |
|--------------------------------------------------|------------------------------------------------------|
| [codec](./codec)                                 | Pluggable codec interface and chain execution engine |
| [codecs/optionalcodec](./codecs/optionalcodec)   | `core/types/optional.Optional[T]` field codec        |
