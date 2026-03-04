# sources

Policy source implementations for the OPA authorization package. Each subpackage implements the `opa.PolicySource` interface for a specific
backend. Sources are passive data fetchers — the Manager handles all polling and change-detection scheduling.

## Packages

| Package                        | Description                                         |
|--------------------------------|-----------------------------------------------------|
| [filesystem](./filesystem)     | Local filesystem policies                            |
| [gitlab](./gitlab)             | GitLab repository policies via API                   |
