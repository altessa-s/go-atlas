# proto

Schema home for the gRPC contracts exposed by `go-atlas`. This directory
holds `.proto` source files; generated bindings are published from CI to
dedicated per-language repositories.

## Public schemas

| Package | File | Notes |
|---------|------|-------|
| `io.altessa.grpc.scheduler.v1` | [`scheduler/v1/scheduler.proto`](scheduler/v1/scheduler.proto) | `SchedulerService` RPC contract |
| `io.altessa.grpc.protovalidate.v1` | [`protovalidate/v1/bad_request.proto`](protovalidate/v1/bad_request.proto) | `BadRequest` / `FieldViolation` error detail messages |

These are mirrored on every `main` push and on every `vX.Y.Z` tag to:

| Language | Repository | Module / coordinates |
|----------|------------|----------------------|
| Go       | [`altessa-s/atlas-proto-gen-go`](https://github.com/altessa-s/atlas-proto-gen-go) | `github.com/altessa-s/atlas-proto-gen-go` |
| Java     | [`altessa-s/atlas-proto-gen-java`](https://github.com/altessa-s/atlas-proto-gen-java) | Maven artifact in GitHub Packages |

## Internal test fixture

`fieldmasktest/v1/test.proto` is a test-only fixture for the
`domain/proto/fieldmask` package. It is **not** published. Its generated
output lives at `proto/gen/fieldmasktest/v1/test.pb.go` and is committed
in this repository.

## Generation

Local generation uses [buf](https://buf.build). Public bindings are
written to `proto/gen/{go,java}/` (gitignored, ephemeral); the test
fixture is written to `proto/gen/fieldmasktest/` (committed).

```
make proto-go        # ephemeral; mirror of what CI publishes to atlas-proto-gen-go
make proto-java      # ephemeral
make proto-testpb    # regenerates the committed fieldmasktest fixture
make proto           # all of the above
```

Each language has its own `buf.gen.<lang>.yaml` template so CI jobs can
provision only the toolchain they need.

## Versioning

- **Tag-driven semver**: a `vX.Y.Z` tag on this repository propagates to
  the matching tag in each language repo via `.github/workflows/proto-publish.yml`.
- **Main snapshot**: every push to `main` that touches `proto/**` updates
  each language repo's `main` branch. Go consumers track snapshots via
  pseudo-versions (`go get @main`); Java publishes a `-SNAPSHOT` artifact.

The Java JAR is published to GitHub Packages on tag pushes only.

## Adding a new schema

1. Create a new directory under `proto/` (e.g., `proto/foo/v1/`).
2. Add `option go_package = "github.com/altessa-s/atlas-proto-gen-go/foo/v1;foov1";` and Java options.
3. Run `make proto` locally to verify generation.
4. Update the publish workflow's `--path` filters if you want this
   schema published. Test-only schemas should be added to
   `buf.gen.testpb.yaml` instead and committed to `proto/gen/`.

## Linting and breaking changes

`.github/workflows/proto-check.yml` runs `buf lint` and
`buf breaking --against` on every PR touching `proto/**`. Breaking-change
violations block the merge; intentional breaks need an explicit
`// buf:lint:ignore` annotation justified in the PR description.
