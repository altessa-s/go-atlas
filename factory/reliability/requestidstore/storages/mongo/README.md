# factory

```go
import "github.com/altessa-s/go-atlas/factory/reliability/requestidstore/storages/mongo"
```

Package `factory` binds a MongoDB client to the AIP-155 storage factory.

## Usage

Register `Builder` with the [request ID storage factory](../..). The caller owns the connection.
