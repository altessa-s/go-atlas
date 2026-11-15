# security

Security subsystem for the Atlas framework. Provides HMAC signing, secret sealing and HashiCorp Vault integration. The factories that build the
secret manager of [go-kryptos](https://github.com/altessa-s/go-kryptos) and the TLS providers of [go-tlsutils](https://github.com/altessa-s/go-tlsutils)
from configuration live in [`factory/ext/kryptos`](../factory/ext/kryptos/) and [`factory/ext/tlsutils`](../factory/ext/tlsutils/).

## Packages

| Package                | Description                                                    |
|------------------------|----------------------------------------------------------------|
| [vault](./vault)       | High-level Vault client with pluggable auth and token renewal  |

## Design principles

- **Backend-agnostic** — secret and TLS providers are injected through interfaces.
- **Secure by default** — TLS 1.2+ enforced, secrets cleared on GC, distributed locking.
- **Factory-driven** — each subsystem has a config-based builder under [`factory/security`](../factory/security/).
