# storages

Audit storage implementations. Each subpackage provides a backend for persisting audit events emitted by the `audit.Auditor`.

## Subpackages

| Package                | Description                       |
|------------------------|-----------------------------------|
| [memory](./memory)     | In-memory backend for dev/test    |
| [mongo](./mongo)       | MongoDB-backed persistent storage |
