# factory

```go
import "github.com/altessa-s/go-atlas/auth/oauth2client/factory"
```

Builds client-side OAuth2 token acquisition from an [`authconfig.OAuth2Client`](../../../config/auth) template and injected dependencies.

`Build` returns a self-refreshing `client_credentials` `oauth2.TokenSource` (the machine-to-machine case). `BuildExchanger` returns an RFC 8693
[`oauth2client.Exchanger`](..) from the same configuration. The token endpoint comes from config `tokenURL`, or — when the config sets `discoveryURL` —
from an OIDC discovery source injected with `UseTokenEndpointSource` (satisfied by [`oidc.Provider`](../../oidc)).

## Builder

| Method                          | Description                                                              |
|---------------------------------|--------------------------------------------------------------------------|
| `New(cfg)`                      | Create a builder for the config. A nil cfg errors at build time.         |
| `UseTokenEndpointSource(src)`   | Inject the OIDC discovery source (needed only when cfg uses `discoveryURL`). |
| `UseHTTPClient(client)`         | Inject the HTTP client for token requests (timeouts, mTLS, tracing).     |
| `UseLogger(logger)`             | Inject the logger for fetch-failure and retry logging. Optional.         |
| `UseMetrics(m)`                 | Inject the metrics sink (`oauth2client.NewMetrics`). Optional.           |
| `UseClientAuth(a)`              | Inject a JWT-assertion authenticator (`PrivateKeyJWT`/`ClientSecretJWT`), overriding config. Optional. |
| `Build(ctx)`                    | Assemble a self-refreshing `client_credentials` `oauth2.TokenSource`.    |
| `BuildExchanger(ctx)`           | Assemble an RFC 8693 `oauth2client.Exchanger`.                           |

The factory also builds the authenticator from the config `clientAuth` block (`method: private_key_jwt | client_secret_jwt`); for `private_key_jwt` it
loads the PEM `privateKey` (a `redacted.RedactedString`, kept out of logs and expandable from a `$__secret{namespace:key}` placeholder by the config
loader) and parses it per `algorithm`. A programmatic `UseClientAuth` wins over config.

## Usage

```go
src, err := factory.New(cfg.OAuth2Client).
    UseTokenEndpointSource(oidcProvider). // only when cfg sets discoveryURL
    Build(ctx)
if err != nil {
    return err
}
creds := client.NewInsecureTokenCredentials(src)
```

## See also

- [`auth/oauth2client`](..) — the underlying grant helpers and token exchanger.
- [`authconfig.OAuth2Client`](../../../config/auth) — the configuration template.
- [`auth/oidc`](../../oidc) — provides the `TokenEndpointSource` via discovery.
