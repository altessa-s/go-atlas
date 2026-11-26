# cache

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"
```

Package `cache` caches unary gRPC responses with per-method policies and optional compression. It accepts any backend implementing
[`cache.Cacher`](../../../../cache).

## Key types

| Type                | Description                                                               |
|---------------------|---------------------------------------------------------------------------|
| `MethodPolicy`      | Per-method response prototype and compression                             |
| `KeyGenerator`      | Overrides the cache storage key                                           |
| `IdentityFunc`      | Resolves a namespace from verified context data                           |
| `IdentityState`     | Whether the call can use a verified or shared namespace                   |
| `DecisionFunc`      | Selects responses to cache and their TTL                                  |
| `Serializer`        | Serializes responses with optional compression                            |
| `MetadataProcessor` | Supplies additional response discriminators from metadata or context      |

## Options

| Option                  | Default                        | Description                                                |
|-------------------------|--------------------------------|------------------------------------------------------------|
| `WithMethod`            | —                              | Register a method with its response prototype              |
| `WithMethodPolicy`      | —                              | Register methods with compression presets                  |
| `WithPublicMethods`     | None                           | Allow shared caching for exact full RPC names              |
| `WithDefaultTTL`        | 5m                             | Default cache TTL (1s to 24h)                               |
| `WithKeyGenerator`      | SHA-256 over the request tuple | Override the storage key while retaining tuple verification |
| `WithIdentityFunc`      | Verified identity and grants   | Customize how verified callers are partitioned             |
| `WithAuthRequired`      | false                          | Bypass caching when verified identity is unavailable       |
| `WithReadMaskHeader`    | `x-goog-fieldmask`             | Include a custom read-mask header alongside the default    |
| `WithCacheDecision`     | Success only with default TTL  | Customize when and how long to cache                       |
| `WithSerializer`        | Protobuf                       | Override response serialization                            |
| `WithCompression`       | None                           | Set custom gzip size and level                             |
| `WithCompressionPreset` | None                           | Use a predefined gzip preset                               |
| `WithCacheHeaders`      | true                           | Add `x-cache` hit or miss headers                          |
| `WithKeysPrefix`        | Empty                          | Add a prefix to storage keys                               |
| `WithMetadataKeys`      | `DefaultMetadataKeys`          | Select metadata response discriminators                    |
| `WithMetadataProcessor` | nil                            | Supply additional response discriminators                  |
| `WithIgnoreMethods`     | —                              | Skip named methods                                         |
| `WithIgnorePatterns`    | Reflection and health          | Skip methods matching regular expressions                  |
| `WithLogger`            | Discard                        | Set the structured logger                                  |

## Verified identity

Dependency-ordered chains run metadata, auth, IP and geographic access controls, and rate limiting before cache. Apply the same ordering when
assembling a chain manually. The default partitions by verified tenant, subject, sorted scopes and roles, and the authentication method and verified
issuer when available. Authenticated calls with an empty subject or unrecognized credentials bypass caching. For a server without auth, register
both the method's response policy and its exact full RPC name with `WithPublicMethods`. Choose methods whose responses can safely be shared among
anonymous callers. In the gRPC server factory, set `interceptors.cache.publicMethods` and register the policies with `UseCacheMethodPolicies`.

Use `WithIdentityFunc` when the default cannot identify the current caller or the response depends on other verified attributes. Return a stable,
nonempty namespace with the `Verified` state. Include the tenant, subject, authenticator or issuer, and every scope, role, or other verified claim
that can change the response. Use only verified data from the current call. Return `AuthenticatedUnknown` if a safe namespace is unavailable.
Return `NoAuth` only for calls eligible to share a namespace without authentication.

Use `WithAuthRequired()` in directly constructed authenticated chains to bypass caching when auth skips a call. The gRPC server factory enables
this option on its config-built cache whenever auth is registered. Auth-ignored public methods bypass that cache. To use a custom identity with the
factory, exclude the config-built cache with `WithInterceptors(cache.ID)` and register a configured cache with `WithInterceptor`.

## Cache keys

The default storage key is 32 hexadecimal characters from SHA-256 over the framed method, identity namespace, serialized request, and response
discriminators. Entries carry a separate SHA-256 verifier over the same tuple, including when a custom storage key is used. A verifier mismatch
is a cache miss. When configured through `WithKeyGenerator`, `NewKeyGenerator` uses the interceptor's resolved namespace, including a custom
`WithIdentityFunc`. Shared entries written before public opt-in become misses after upgrading. Cached gRPC errors preserve their full status,
including structured details.

The default metadata covers API version, feature flags, and the read mask. Caller identity comes from verified context. The default read-mask
header is always included, even when `WithMetadataKeys` replaces the default list. For a different header, set
`WithReadMaskHeader`. The gRPC server factory uses the registered field-mask interceptor's effective header, including code-level overrides.

Method names and processor-output keys are case-sensitive. Protobuf requests use deterministic serialization, including map fields. Metadata is
bounded to 100 selected keys, 256 bytes per key, 20 values per key, and 1024 bytes per value. Processor output uses the same key and value bounds.
Exceeding a bound returns `ErrMetadataTooLarge` and bypasses caching. A custom metadata processor receives an empty writable map if the request
has no incoming metadata.

## Subpackages

| Package                      | Description                          |
|------------------------------|--------------------------------------|
| [compression](./compression) | Gzip compression for cache entries   |
