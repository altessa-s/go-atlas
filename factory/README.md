# factory

The composition layer of go-atlas: every package under `factory/` maps a configuration schema (`config/*`) to a ready-to-use component. Runtime
packages never import a factory; a factory may import runtime packages, schemas and other factories.

## Layout

| Path                                | Builds                                                                                      |
|-------------------------------------|---------------------------------------------------------------------------------------------|
| `factory/<path>`                    | The go-atlas package at `<path>` (`factory/cache` builds `cache`)                 |
| `factory/<path>/storages/x`         | The `x` backend of `<path>`, registered on the parent factory (`UseStorage`, `UseProvider`) |
| [`core`](core/)                     | Base type shared by the builders (`Base`, `RequireDependency`, `WrapError`)                 |
| [`infrastructure`](infrastructure/) | MongoDB, Redis, NATS, ClickHouse and Meilisearch clients                                    |
| [`httpclient`](ext/net/httpclient/)         | The go-atlas-net HTTP client; [`observe`](ext/net/httpclient/observe/) adapts its observer          |
| [`proxydial`](ext/net/proxydial/)           | go-atlas-net forward-proxy dialers for the HTTP and gRPC clients                            |
| [`horai`](ext/horai/)                   | The go-horai scheduler and its ClickHouse, MongoDB and Redis storages                       |
| [`kryptos`](ext/kryptos/)               | The go-kryptos secret manager and its GCP, Lockbox and Vault providers                      |
| [`slogx`](ext/slogx/)                   | go-slogx loggers                                                                            |
| [`tlsutils`](ext/tlsutils/)             | go-tlsutils TLS providers and their S3, Vault and SPIFFE sources                            |

A factory for a component that lives in a split-out module is named after that module. A factory that builds a driver backend or wraps a heavy
SDK stays its own package, so an application that does not register that backend never links its driver.

## Errors

Builders return errors matchable with `errors.Is`: a missing configuration is `ErrConfigRequired`, a missing dependency `ErrDependencyRequired`,
a known backend without a registered builder `ErrNotRegistered`, and an unknown type or provider `ErrUnsupported`. The sentinels live in
[`core`](core/README.md#errors); the message text names the offending item and is not a stable contract.
