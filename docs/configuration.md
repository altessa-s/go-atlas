[← Architecture](architecture.md) · [Back to README](../README.md) · [Metrics Reference →](metrics.md)

# Configuration

go-atlas provides a flexible configuration system through the `config/loader` package.

## Overview

The loader supports multiple sources, merged in order of precedence:

1. **Default values** — struct tags (`default:"value"`)
2. **Configuration files** — YAML or TOML
3. **Environment variables** — struct tags (`env:"VAR_NAME"`)
4. **Secret expansion** — `$__secret{namespace:key}` syntax

## Basic Usage

```go
type Config struct {
    Database struct {
        Host string `yaml:"host" default:"localhost" env:"DB_HOST"`
        Port int    `yaml:"port" default:"5432" env:"DB_PORT"`
    } `yaml:"database"`
}

cfg := &Config{}
p := loader.New(&yaml3.Backend{}, loader.WithPath("config.yaml"))
p.Load(cfg)
```

## Configuration File

```yaml
# config.yaml
database:
  host: ${DB_HOST}
  port: 5432
  password: $__secret{myapp:db_password}

server:
  address: ":8080"
  read_timeout: 30s
```

## Environment Variables

Environment variables are mapped to struct fields using the `env` tag:

```go
type Config struct {
    Host string `yaml:"host" env:"APP_HOST"`
    Port int    `yaml:"port" env:"APP_PORT"`
}
```

Nested structures use the `__` delimiter in environment variable names:

```bash
export DATABASE__HOST=localhost
export DATABASE__PORT=5432
```

## Variable Substitution

Use `${VAR}` or `${VAR:default}` syntax in configuration files:

```yaml
database:
  host: ${DB_HOST:localhost}
  port: ${DB_PORT:5432}
```

## Secret Expansion

The `$__secret{namespace:key}` syntax enables secure secret injection from external providers (Vault, GCP Secret Manager, etc.):

```yaml
database:
  password: $__secret{myapp:db_password}
  api_key: $__secret{external:api_key}
```

Configure the secrets manager:

```go
manager, _ := secrets.New[string](provider)
p := loader.New(nil,
    loader.WithPath("config.yaml"),
    loader.WithSecretsManager(manager),
)
```

Supported secret providers:

| Provider | Package |
|----------|---------|
| HashiCorp Vault | `security/vault` |
| GCP Secret Manager | `security/secrets/providers/gcp` |
| In-memory (testing) | `security/secrets/providers/memory` |

## Backends

| Backend | Format | Package |
|---------|--------|---------|
| YAML v3 | `.yaml`, `.yml` | `config/loader/backends/yaml3` |
| TOML | `.toml` | `config/loader/backends/toml` |

## Validation

The loader supports automatic validation via struct tags. Validation runs after all sources are merged:

```go
type Config struct {
    Port int `yaml:"port" validate:"required,min=1,max=65535"`
}
```

## Configuration Templates

The `config/templates` package provides pre-built configuration structs for common components:

- HTTP server configuration
- gRPC server configuration
- MongoDB connection settings
- Redis connection settings
- NATS connection settings
- Observability settings (tracing, metrics, logging)
