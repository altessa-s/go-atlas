# pagingconfig

```go
import pagingconfig "github.com/altessa-s/go-atlas/config/paging"
```

Package `pagingconfig` defines the schema of signed page tokens shared by every paginated component: audit queries (`audit.paging`), scheduler
lists (`scheduler.paging`) and service List RPCs.
[`factory/ext/aip/pagination/pagetoken`](../../factory/ext/aip/pagination/pagetoken) builds a go-aip `pagetoken.Codec` from it; runtime
packages never import it.

## Key types

| Type          | Description                                                                                           |
|---------------|-------------------------------------------------------------------------------------------------------|
| `Config`      | Signing key, previous keys, key encoding, token lifetime and encryption; `Keys` decodes the keys.     |
| `Encryption`  | `enabled` issues encrypted tokens; `strict` (requires `enabled`) also rejects signed ones.            |
| `KeyEncoding` | `raw` (the string bytes, default) or `base64` (standard padded base64).                               |
| `KeyRule`     | Validation rule for a key kept under another schema's field name: decodes, `MinKeyLength` (32 bytes). |
| `DecodeKey`   | Turns a configured key into key bytes (`ErrKeyEncoding` when it does not decode).                     |

## YAML

| Key                  | Type     | Default | Description                                                            |
|----------------------|----------|---------|------------------------------------------------------------------------|
| `signingKey`         | string   | —       | Required. At least 32 bytes after decoding, the same on every replica. |
| `previousKeys`       | []string | —       | Retired non-empty keys still verifying tokens; keep for a `tokenTTL`.  |
| `keyEncoding`        | string   | `raw`   | `raw` or `base64`; applies to every key.                               |
| `tokenTTL`           | duration | `24h`   | Token lifetime; `0` keeps the codec default.                           |
| `encryption.enabled` | bool     | `false` | Issue encrypted tokens; signed ones are still accepted.                |
| `encryption.strict`  | bool     | `false` | Also reject signed tokens; requires `enabled`.                         |

Roll encryption out in two steps: set `encryption.enabled` on every replica, then `encryption.strict` once the signed tokens in circulation have
expired (one `tokenTTL`). The encryption keys are derived from the signing keys, so `previousKeys` rotates them too.

Errors name the offending field and never quote a key. See the [config index](../README.md) for the other schema packages.
