# factory

```go
import "github.com/altessa-s/go-atlas/factory/ext/aip/pagination/pagetoken"
```

Package `factory` builds a go-aip [`pagetoken.Codec`](https://github.com/altessa-s/go-aip/tree/develop/pagination/pagetoken) from the shared paging schema
[`config/paging`](../../../../../config/paging): it
decodes the signing and previous keys under the configured `keyEncoding` and maps `tokenTTL` and the `encryption` block. Audit queries, scheduler lists
and service List RPCs build their page-token codecs here.

## Key types

| Type           | Description                                                                 |
|----------------|-----------------------------------------------------------------------------|
| `CodecBuilder` | `New(cfg).UseOptions(...).Build()` returns the codec or a decoding error. |

## Mapping

| Schema field         | Codec                                                   |
|----------------------|---------------------------------------------------------|
| `signingKey`         | Signing key passed to `pagetoken.New`.                     |
| `previousKeys`       | `pagetoken.WithPreviousKeys`.                              |
| `keyEncoding`        | How every key is decoded (`raw` or `base64`).           |
| `tokenTTL`           | `pagetoken.WithTTL` when positive; `0` keeps `DefaultTTL`. |
| `encryption.enabled` | `pagetoken.WithEncryption`.                                |
| `encryption.strict`  | `pagetoken.WithStrictEncryption`.                          |

## Usage

```go
codec, err := factory.New(cfg.Paging).Build()
if err != nil {
    return err
}
```
