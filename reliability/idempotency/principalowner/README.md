# principalowner

```go
import "github.com/altessa-s/go-atlas/reliability/idempotency/principalowner"
```

This adapter binds [`reliability/idempotency`](..) records to the verified tenant and subject carried by
[`auth/principal`](../../../auth/principal).

## Key types

| Type or function | Description |
|------------------|-------------|
| `PrincipalOwner` | Returns an `idempotency.OwnerFunc` for the verified principal in the context. Missing principal or subject returns an empty owner. |

## Usage

Use the adapter when constructing a keeper directly. The configuration factory uses `PrincipalOwner` by default.

```go
import (
    "github.com/altessa-s/go-atlas/reliability/idempotency/principalowner"
    idempotency "github.com/altessa-s/go-atlas/reliability/idempotency"
)

keeper := idempotency.New(storage,
    idempotency.WithOwner(principalowner.PrincipalOwner),
)
```

`reliability/idempotency` stays independent of authentication packages; the adapter composes its owner callback with the authenticated identity.
