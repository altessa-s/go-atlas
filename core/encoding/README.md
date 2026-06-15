# encoding

Encoding, hashing, and serialization utilities for the Atlas framework. Each subpackage can be imported independently with zero external deps.

## Subpackages

| Package                        | Description                                              |
|--------------------------------|----------------------------------------------------------|
| [etag](./etag)                 | HTTP/gRPC entity-tags (ETags, RFC 7232 / AIP-154)        |
| [hash](./hash)                 | Deterministic, hex-encoded SHA-256 digests               |
| [serializer](./serializer)     | Format-agnostic `Serializer` interface with JSON default |
