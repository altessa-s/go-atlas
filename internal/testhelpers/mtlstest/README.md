# mtlstest

```go
import "github.com/altessa-s/go-atlas/internal/testhelpers/mtlstest"
```

The HTTP and gRPC integration tests use one in-memory CA to issue a server certificate for an IP address and a client certificate with a SPIFFE URI.

## Usage

```go
ca := mtlstest.NewCA(t)
server := ca.ServerCert(t, net.IPv4(127, 0, 0, 1))
client := ca.ClientCert(t, "spiffe://example.org/sa/billing")
```
