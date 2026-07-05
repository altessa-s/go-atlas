# serviceinfo

```go
import "github.com/altessa-s/go-atlas/transport/grpc/handlers/serviceinfo"
```

Package `serviceinfo` implements the gRPC `ServiceInfoService` from
[`github.com/altessa-s/proto-gen-go/io/altessa/serviceinfo/v1`](https://pkg.go.dev/github.com/altessa-s/proto-gen-go/io/altessa/serviceinfo/v1).
The `Handler` reports name, description, identifiers, semantic version, build
details, leader status, uptime, and arbitrary metadata. The static portion is
built once in `New` from `core/runtime/appinfo` and protobuf-cloned on every
request to keep callers isolated from each other.

## Key types

| Type / Interface | Description                                                                                |
|------------------|--------------------------------------------------------------------------------------------|
| `Handler`        | Implements `ServiceInfoServiceServer` with options-driven configuration                    |
| `LeaderProvider` | Two-method interface (`IsLeader`, `LeaderId`) — `data/leadelect.Leader` satisfies it natively |
| `LeaderFunc`     | `func(ctx) (isLeader bool, leaderID string)` accepted by `WithLeader` for ad-hoc sources   |

## RPCs

| Method | Type  | Description                                                  |
|--------|-------|--------------------------------------------------------------|
| `Get`  | Unary | Returns service metadata; static fields cached, dynamic fields recomputed per call |

## Options

| Option                  | Default        | Purpose                                                                |
|-------------------------|----------------|------------------------------------------------------------------------|
| `WithServiceName`       | `appinfo.Name` | Override `service_name`                                                |
| `WithServiceDescription`| `""`           | Populate `service_description` (omitted when empty)                    |
| `WithServiceID`         | `""`           | Populate `service_id` and use as `leader_id` when this instance is leader |
| `WithLeaderProvider`    | `nil`          | Plug in a `LeaderProvider` (e.g. `*data/leadelect.Leader`)             |
| `WithLeader`            | `nil`          | Functional shortcut: provide leader state via a single closure         |
| `WithExtraMetadata`     | `nil`          | Merge extra keys into `metadata`; caller keys win on conflict          |

`WithLeader` and `WithLeaderProvider` are mutually exclusive — the last
applied option wins.

The handler does not populate `service_info.proto_version` — surface the
proto module version (or any other build-time dependency version)
through `WithExtraMetadata` instead.

## Usage

```go
import (
    "github.com/altessa-s/go-atlas/data/leadelect"
    "github.com/altessa-s/go-atlas/transport/grpc/handlers/serviceinfo"
)

handler := serviceinfo.New(
    serviceinfo.WithServiceID(node.ID()),
    serviceinfo.WithServiceDescription("Customer billing API"),
    serviceinfo.WithLeaderProvider(electedLeader), // *leadelect.Leader
    serviceinfo.WithExtraMetadata(map[string]string{
        "region":        "eu-west-1",
        "proto_version": protoBuildInfo.Version,
    }),
)
handler.Register(grpcServer, stopCh)
```

For services without `data/leadelect`, plug in a function:

```go
serviceinfo.WithLeader(func(ctx context.Context) (bool, string) {
    return raft.IsLeader(), raft.LeaderID()
})
```
