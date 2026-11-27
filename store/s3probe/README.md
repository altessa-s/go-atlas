# s3probe

```go
import "github.com/altessa-s/go-atlas/store/s3probe"
```

Checks that an S3-compatible bucket is reachable with a pair of access keys. The only operation is a signed `HeadBucket` request with
path-style addressing, sent through the guarded HTTP client of go-atlas-net (SSRF guard and egress policy). Request signing reuses the
SigV4 signer of aws-sdk-go-v2, which the module already depends on.

## Quick Start

```go
p := s3probe.New(policy) // *httpclient.EgressPolicy, may be nil

res, err := p.HeadBucket(ctx, s3probe.Request{
    Endpoint:  "https://s3.example.com",
    Bucket:    "reports",
    AccessKey: accessKey,
    SecretKey: secretKey,
    Region:    "",   // empty means us-east-1
    Timeout:   5 * time.Second,
    Trusted:   true, // stored configuration; false for a draft
})
```

`res.StatusCode` is the HTTP status of the answer. Redirects are not followed and nothing is retried. Mapping a status to a reason is up to
the caller.

## Errors

| Error                | Meaning                                                                      |
|----------------------|------------------------------------------------------------------------------|
| `ErrInvalidRequest`  | request could not be constructed or validated for sending                   |
| `ErrPolicyRefused`   | the egress policy refused the destination before connecting (`KindPolicy`)   |
| `ErrUnreachable`     | name not resolved, connection refused or any other failure to connect        |
| `ErrTLS`             | handshake or certificate failure                                             |
| `ErrTimeout`         | the time limit ran out                                                       |

The transport sentinels match a `*TransportError` through `errors.Is`; `Kind` carries the same distinction. A canceled context is returned
unchanged.

## Egress

- Trusted: the configured policy applies. Private, loopback, link-local and reserved addresses need an allow rule with a CIDR; a deny rule
  wins over an allow rule.
- Untrusted: only public addresses outside the policy's deny rules. Allow rules have no effect.
