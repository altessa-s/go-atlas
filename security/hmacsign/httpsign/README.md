# httpsign

```go
import "github.com/altessa-s/go-atlas/security/hmacsign/httpsign"
```

The `net/http` adapter for [`hmacsign`](..). It reads the raw request body once, verifies the provider's signature header against
it, and restores the body so a downstream handler can read it again. The core `hmacsign` package stays transport-free; this thin
package is the only one that imports `net/http`.

## API

| Function                          | Description                                                                                   |
|-----------------------------------|-----------------------------------------------------------------------------------------------|
| `Middleware(v, opts...)`          | `func(http.Handler) http.Handler` that verifies before the next handler runs.                 |
| `Verify(v, r, opts...)`           | Verifies `r` and returns the raw body, for handlers that prefer an explicit call.             |

## Options

| Option                        | Description                                                                                  |
|-------------------------------|----------------------------------------------------------------------------------------------|
| `WithMaxBytes(int64)`         | Cap on the body read before verification. Default 1 MiB; a larger body yields `ErrBodyTooLarge`. |
| `WithErrorHandler(ErrorHandler)` | Response written on failure (middleware only). Default: 413 for an oversized body, 401 otherwise. |

## Usage

```go
v := hmacsign.NewVerifier(hmacsign.Stripe(), webhookSecret)
mux.Handle("/webhooks/stripe", httpsign.Middleware(v)(stripeHandler))
```

The handler runs only for authentic bodies and reads them from `r.Body` as usual — the middleware has already consumed and restored
them. For an explicit check inside a handler:

```go
body, err := httpsign.Verify(v, r)
if err != nil {
    http.Error(w, "unauthorized", http.StatusUnauthorized)
    return
}
event := parse(body)
```

## See also

- [`hmacsign`](..) — the transport-free signer/verifier and the `Scheme` definitions.
