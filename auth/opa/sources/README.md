# sources

Policy source implementations for the OPA authorization package. Each subpackage implements the `opa.PolicySource` interface for a specific
backend. Sources are passive data fetchers — the Manager handles all polling and change-detection scheduling.

## Packages

| Package                        | Description                                         |
|--------------------------------|-----------------------------------------------------|
| [embed](./embed)               | Embedded fs.FS policies (compiled into binary)       |
| [filesystem](./filesystem)     | Local filesystem policies                            |
| [gitlab](./gitlab)             | GitLab repository policies via API                   |
| [s3](./s3)                     | S3-compatible object store policies                  |
