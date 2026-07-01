# factory

```go
import "github.com/altessa-s/go-atlas/auth/oauth2client/factory"
```

Builds client-side OAuth2 token acquisition from a [`config.OAuth2Client`](../../../config) template and injected dependencies.

`Build` returns a self-refreshing `client_credentials` `oauth2.TokenSource` (the machine-to-machine case). `BuildExchanger` returns an
RFC 8693 [`oauth2client.Exchanger`](..) from the same configuration. The token endpoint comes from config `tokenUrl`, or — when the
config sets `discoveryUrl` — from an OIDC discovery source injected with `UseTokenEndpointSource` (satisfied by
[`oidc.Provider`](../../oidc)).

## Builder

| Method                          | Description                                                              |
|---------------------------------|--------------------------------------------------------------------------|
| `New(cfg)`                      | Create a builder for the config. A nil cfg errors at build time.         |
| `UseTokenEndpointSource(src)`   | Inject the OIDC discovery source (needed only when cfg uses `discoveryUrl`). |
| `UseHTTPClient(client)`         | Inject the HTTP client for token requests (timeouts, mTLS, tracing).     |
| `UseLogger(logger)`             | Inject the logger for fetch-failure and retry logging. Optional.         |
| `UseMetrics(m)`                 | Inject the metrics sink (`oauth2client.NewMetrics`). Optional.           |
| `UseClientAuth(a)`              | Inject a JWT-assertion authenticator (`PrivateKeyJWT`/`ClientSecretJWT`), overriding config. Optional. |

The factory also builds the authenticator from the config `clientAuth` block (`method: private_key_jwt | client_secret_jwt`); for
`private_key_jwt` it loads the PEM `privateKey` (a `Secret`, so `file:`/`env:`/`vault:` refs work) and parses it per `algorithm`. A
programmatic `UseClientAuth` wins over config.
| `Build(ctx)`                    | Assemble a self-refreshing `client_credentials` `oauth2.TokenSource`.    |
| `BuildExchanger(ctx)`           | Assemble an RFC 8693 `oauth2client.Exchanger`.                           |

## Usage

```go
src, err := factory.New(cfg.OAuth2Client).
    UseTokenEndpointSource(oidcProvider). // only when cfg sets discoveryUrl
    Build(ctx)
if err != nil {
    return err
}
creds := client.NewInsecureTokenCredentials(src)
```

## See also

- [`auth/oauth2client`](..) — the underlying grant helpers and token exchanger.
- [`config.OAuth2Client`](../../../config) — the configuration template.
- [`auth/oidc`](../../oidc) — provides the `TokenEndpointSource` via discovery.
