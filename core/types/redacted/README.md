# redacted

```go
import "github.com/altessa-s/go-atlas/core/types/redacted"
```

Package `redacted` provides `RedactedString`, a named `string` type for credentials and other sensitive fields that substitutes its value with the
fixed placeholder `<redacted>` in every standard output context — `fmt`, `log/slog`, `encoding/json`, `gopkg.in/yaml.v3`, `encoding.TextMarshaler`,
and `go.mongodb.org/mongo-driver/v2/bson`. The plain text is reachable only through the explicit `Expose` accessor.

`RedactedString` keeps `reflect.Kind == reflect.String`, so the repo configuration loader, `yaml.v3` scalar decoding, and the Mongo v2 driver all
assign string values into it via reflection — the explicit `Unmarshal*` methods exist for symmetry and for interface-dispatched code paths.

## Methods

| Method                                     | Description                                                                       |
|--------------------------------------------|-----------------------------------------------------------------------------------|
| `String() string`                          | `"<redacted>"` (fmt.Stringer)                                                     |
| `GoString() string`                        | `"RedactedString{<redacted>}"` (fmt.GoStringer; used by `%#v`)                    |
| `LogValue() slog.Value`                    | `slog.StringValue("<redacted>")` (slog.LogValuer)                                 |
| `Expose() string`                          | Underlying plain text — the **only** path to the secret                           |
| `IsEmpty() bool`                           | `s == ""`                                                                         |
| `IsZero() bool`                            | Same as `IsEmpty`; lets `bson:",omitempty"` strip empty fields                    |
| `SecureString() *strings.SecureString`     | Convert to a pooled, memory-zeroing `SecureString` — caller owns `Clear()`        |
| `MarshalJSON() ([]byte, error)`            | `"<redacted>"` (JSON string)                                                      |
| `UnmarshalJSON([]byte) error`              | Accepts any JSON string, stores it verbatim into the underlying value             |
| `MarshalYAML() (any, error)`               | `"<redacted>"` scalar (yaml.v3)                                                   |
| `UnmarshalYAML(*yaml.Node) error`          | Decodes a scalar string into the underlying value                                 |
| `MarshalText() ([]byte, error)`            | `[]byte("<redacted>")`                                                            |
| `UnmarshalText([]byte) error`              | Stores incoming bytes verbatim                                                    |
| `MarshalBSONValue() (byte, []byte, error)` | BSON string `<redacted>`                                                          |
| `UnmarshalBSONValue(byte, []byte) error`   | Decodes a BSON string into the underlying value                                   |

## Serialization

`RedactedString` implements `json.Marshaler` / `json.Unmarshaler`, `yaml.Marshaler` / `yaml.Unmarshaler` (`gopkg.in/yaml.v3`),
`encoding.TextMarshaler` / `encoding.TextUnmarshaler`, and `bson.ValueMarshaler` / `bson.ValueUnmarshaler`
(`go.mongodb.org/mongo-driver/v2`). `Marshal` always emits the placeholder `<redacted>`; `Unmarshal` stores the incoming string verbatim, so a value
loaded from YAML/JSON/BSON is exposed by `Expose()` but re-serializing the same struct redacts it again. `IsZero` returning `true` for the empty
string makes `bson:",omitempty"` strip empty fields entirely from the on-the-wire document.

Because the BSON marshallers must live on the type, this package depends on `go.mongodb.org/mongo-driver/v2/bson` — the same trade-off made by
[`core/types/optional`](../optional/README.md).

## Usage

```go
type DatabaseConfig struct {
    URI redacted.RedactedString `yaml:"uri" json:"uri" bson:"uri"`
}

cfg := DatabaseConfig{URI: "mongodb://user:pass@host/db"}

slog.Info("connecting", "cfg", cfg)
// level=INFO msg=connecting cfg.uri=<redacted>

raw, _ := json.Marshal(cfg)
// raw == {"uri":"<redacted>"}

conn := cfg.URI.Expose() // intentional, plain text
client, _ := mongo.Connect(options.Client().ApplyURI(conn))
```

## When to use

- Struct fields for credentials, tokens, URIs containing embedded passwords, API keys, signing seeds.
- Any field that must never accidentally end up in a log line, a JSON dump, a stack trace, or an error message.

## When NOT to use

- **Credentials that must be zeroed from memory after use** — use [`core/text/strings.SecureString`](../../text/strings) directly, or call
  `RedactedString.SecureString()` for a one-shot bridge.
- **Already-public identifiers** that just happen to look like secrets (e.g. account IDs). `RedactedString` actively prevents inspection, which is
  the wrong default for non-secret data.
- **Generic optionality.** Reach for [`core/types/optional`](../optional/README.md) when the question is "present or absent", not "redact in output".
