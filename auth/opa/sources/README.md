# sources

Policy source implementations for the OPA authorization package. Each subpackage implements the `opa.PolicySource` interface for a specific
backend, providing policy fetching and optional hot-reload support.

## Packages

| Package                        | Description                                         |
|--------------------------------|-----------------------------------------------------|
| [filesystem](./filesystem)     | Local filesystem policies with fsnotify hot-reload   |
