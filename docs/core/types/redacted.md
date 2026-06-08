# RedactedString

Named `string` type for credential and other sensitive fields that substitutes its value with the fixed placeholder `<redacted>` in every standard
output context — `fmt`, `log/slog`, `encoding/json`, `gopkg.in/yaml.v3`, `encoding.TextMarshaler`, and `go.mongodb.org/mongo-driver/v2/bson`. The
underlying plain text is reachable only through the explicit `Expose` accessor.

```go
import "github.com/altessa-s/go-atlas/core/types/redacted"
```

`RedactedString` keeps `reflect.Kind == reflect.String`. The repo configuration loader, `yaml.v3` scalar decoding, and the Mongo v2 BSON driver
populate it through reflection without invoking any `Unmarshal*` method — the explicit `Unmarshal*` methods exist for symmetry and for interface-
dispatched code paths.

> **Not a secure-memory primitive.** `RedactedString` is a cheap log/marshal guard. For credentials that must be zeroed from memory after use,
> reach for [`core/text/strings.SecureString`](../text/strings.md) directly — or call `RedactedString.SecureString()` for a one-shot bridge.

---

## API overview

| Symbol group  | Members                                                                                                              |
|---------------|----------------------------------------------------------------------------------------------------------------------|
| Stringers     | `String`, `GoString`                                                                                                 |
| Logging       | `LogValue`                                                                                                           |
| Accessors     | `Expose`, `IsEmpty`, `IsZero`                                                                                        |
| Conversions   | `SecureString`                                                                                                       |
| Serialization | `MarshalJSON`, `UnmarshalJSON`, `MarshalYAML`, `UnmarshalYAML`, `MarshalText`, `UnmarshalText`, `MarshalBSONValue`, `UnmarshalBSONValue` |

---

## Stringers and logging

| Method                  | Returns                                            |
|-------------------------|----------------------------------------------------|
| `String() string`       | `"<redacted>"` (fmt.Stringer; `%v`, `%s`, `%+v`)   |
| `GoString() string`     | `"RedactedString{<redacted>}"` (fmt.GoStringer; `%#v`) |
| `LogValue() slog.Value` | `slog.StringValue("<redacted>")` (slog.LogValuer)  |

```go
s := redacted.RedactedString("mongodb://user:pass@host/db")

fmt.Sprintf("%v %s %#v", s, s, s)
// "<redacted> <redacted> RedactedString{<redacted>}"

slog.Info("connecting", "uri", s)
// level=INFO msg=connecting uri=<redacted>
```

---

## Accessors

| Method            | Returns                                                                              |
|-------------------|--------------------------------------------------------------------------------------|
| `Expose() string` | Underlying plain text — the **only** path to the secret                              |
| `IsEmpty() bool`  | `s == ""`                                                                            |
| `IsZero() bool`   | Same as `IsEmpty`; lets `bson:",omitempty"` strip empty fields                       |

`Expose` is deliberately verbose — every call site that wants the plain value is a grep-target during security review.

```go
client, err := mongo.Connect(options.Client().ApplyURI(cfg.URI.Expose()))
```

---

## Conversions

### SecureString

`SecureString() *strings.SecureString` converts the redacted value into a pooled, memory-zeroing
[`core/text/strings.SecureString`](../text/strings.md). The caller owns `Clear()`:

```go
ss := cfg.Password.SecureString()
defer ss.Clear()
useSecret(ss)
```

---

## Serialization

`RedactedString` implements `json.Marshaler` / `json.Unmarshaler`, `yaml.Marshaler` / `yaml.Unmarshaler` (`gopkg.in/yaml.v3`),
`encoding.TextMarshaler` / `encoding.TextUnmarshaler`, and `bson.ValueMarshaler` / `bson.ValueUnmarshaler`
(`go.mongodb.org/mongo-driver/v2`). `Marshal` always emits the placeholder `<redacted>`; `Unmarshal` stores the incoming string verbatim — so a
value loaded from YAML/JSON/BSON is exposed by `Expose()` but re-serializing the same struct redacts it again.

| Direction       | `RedactedString("secret")`  | Empty (`""`)                 | Notes                                                                       |
|-----------------|-----------------------------|------------------------------|-----------------------------------------------------------------------------|
| JSON marshal    | `"<redacted>"`              | `"<redacted>"`               | `encoding/json` HTML-escapes `<` and `>` to `<` / `>` on the wire |
| JSON unmarshal  | `RedactedString("secret")`  | `RedactedString("")`         | Non-string JSON values (object, number, bool, null) are rejected            |
| YAML marshal    | `<redacted>\n`              | `<redacted>\n`               | `gopkg.in/yaml.v3`                                                          |
| YAML unmarshal  | `RedactedString("secret")`  | `RedactedString("")`         | Scalar string only; sequence/mapping nodes are rejected                     |
| Text marshal    | `[]byte("<redacted>")`      | `[]byte("<redacted>")`       | Used by `url.Values`, `http.Header`, log/slog text fallback                 |
| Text unmarshal  | `RedactedString("secret")`  | `RedactedString("")`         | Stores incoming bytes verbatim                                              |
| BSON marshal    | BSON string `<redacted>`    | omitted (with `omitempty`)   | `IsZero` returning `true` for `""` triggers `bson:",omitempty"`             |
| BSON unmarshal  | `RedactedString("secret")`  | `RedactedString("")`         | BSON string only; other BSON types are rejected                             |

```go
type Database struct {
    URI      redacted.RedactedString `yaml:"uri" json:"uri" bson:"uri"`
    Password redacted.RedactedString `bson:"password,omitempty"`
}

raw, _ := bson.Marshal(Database{}) // password is omitted via IsZero+omitempty.

var loaded Database
_ = yaml.Unmarshal([]byte(`uri: mongodb://user:pass@host/db`), &loaded)
loaded.URI.Expose()                // "mongodb://user:pass@host/db"

out, _ := json.Marshal(loaded)     // {"uri":"<redacted>","Password":"<redacted>"}
```

Because the BSON marshallers must live on the type, `core/types/redacted` depends on `go.mongodb.org/mongo-driver/v2/bson` — the same trade-off
made by [`core/types/optional`](optional.md). The benefit is that `RedactedString` is a first-class Mongo field type without forcing a wrapper at
every call site.

---

## When to use

- **Credential fields** in configuration structs, request/response DTOs, and persisted documents: passwords, tokens, API keys, signing seeds.
- **URIs with embedded secrets** (`mongodb://user:pass@host`, `redis://:password@…`, `https://user:apikey@…`). Redact the entire URI even though
  parts are harmless — partial redaction is easy to get wrong.
- **Anywhere a field must never reach a log line, a JSON dump, an error message, or a stack trace** without an explicit `Expose()` call.

## When NOT to use

- **Already-public identifiers** that just look like secrets (account IDs, tenant slugs). `RedactedString` actively prevents inspection, which is
  the wrong default for non-secret data.
- **Credentials that must be zeroed from memory after use.** Use [`core/text/strings.SecureString`](../text/strings.md) directly, or bridge with
  `RedactedString.SecureString()` followed by `defer ss.Clear()`.
- **Generic optionality.** Reach for [`Optional[T]`](optional.md) when the question is "present or absent", not "redact in output".
- **Programmatic comparisons against the underlying value.** Use `s.Expose() == "expected"` rather than relying on `==` against an untyped
  constant (which works only because the underlying kind is `string` — easy to break later).

---

## Performance notes

- **Zero allocations on hot paths.** `String`, `GoString`, `LogValue`, `Expose`, `IsEmpty`, `IsZero` and `MarshalYAML` are all `0 B/op` /
  `0 allocs/op` (measured at ~1.6 ns/op on Apple M4 Pro).
- **`MarshalText` allocates once** (~16 B) to return the placeholder as a fresh `[]byte` — `encoding.TextMarshaler`'s contract forbids returning
  a shared backing array.
- **`MarshalJSON` allocates once** (~24 B) for the quoted JSON literal; nothing in the hot path constructs a `json.Encoder`.
- **`MarshalBSONValue` / `UnmarshalBSONValue`** delegate to `bson.MarshalValue` / `bson.UnmarshalValue` — allocation cost is bounded by the BSON
  driver, not by this package.

---

## Relationship with `config.Secret`

`config.Secret` is a type alias for `redacted.RedactedString`:

```go
// In config/secret.go.
type Secret = redacted.RedactedString
```

The two names refer to **the same type** — methods, comparability, and serialization behavior are identical. Pick whichever spelling reads
better at the call site:

- **Inside `config/`** or in code that already imports the `config` package: `config.Secret` keeps configuration structs terse
  (`Password Secret \`yaml:"password"\``) and avoids an extra import.
- **In packages that have no other reason to depend on `config`** (a `data/` repository, a `transport/` handler, a domain entity): use
  `redacted.RedactedString` directly to keep the import graph tight.

```go
// Inside config/, the local alias is the natural choice.
type Redis struct {
    Password Secret `yaml:"password"`
}

// Outside config/, depend on core/types/redacted directly.
import "github.com/altessa-s/go-atlas/core/types/redacted"

type Cache struct {
    APIKey redacted.RedactedString
}
```

---

## See also

- [optional.md](optional.md) — sibling type for "value or absent".
- [result.md](result.md) — sibling type for "value or error".
- `core/types/redacted/README.md` — package-level reference next to the source.
- [`core/text/strings.SecureString`](../text/strings.md) — pooled, memory-zeroing companion for credentials that must be wiped after use.
