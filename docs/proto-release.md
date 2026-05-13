# Proto release process

How gRPC contracts in [`proto/`](../proto/) reach external Go and Java
consumers, and how to ship schema changes safely.

---

## Overview

`go-atlas` is the **schema owner** — humans edit `.proto` files here and
nowhere else. Generated bindings live in dedicated upstream repositories,
one per language, and are populated automatically by CI:

| Language | Repository | Consumed as |
|----------|------------|-------------|
| Go       | [`altessa-s/atlas-proto-gen-go`](https://github.com/altessa-s/atlas-proto-gen-go) | Go module `github.com/altessa-s/atlas-proto-gen-go` |
| Java     | [`altessa-s/atlas-proto-gen-java`](https://github.com/altessa-s/atlas-proto-gen-java) | Maven artifact `io.altessa.grpc:atlas-proto-gen-java` (GitHub Packages) |

The `fieldmasktest/v1` schema is a test fixture for `domain/proto/fieldmask`
and stays inside go-atlas: source under `proto/fieldmasktest/`, generated
output committed at `proto/gen/fieldmasktest/v1/test.pb.go`. It is excluded
from every publish pipeline.

go-atlas's own Go consumers (`transport/grpc/handlers/scheduler` and
`transport/grpc/interceptors/protovalidator/buf`) import from
`atlas-proto-gen-go` just like external services — the toolkit is its own
biggest consumer of the bindings.

## Versioning

- **Tag-driven semver.** A `vX.Y.Z` tag on `go-atlas` propagates to a
  matching `vX.Y.Z` tag on each target repository. The Java JAR is
  published to GitHub Packages at the same version.
- **Main snapshot.** Every push to `main` that touches `proto/**`
  republishes the snapshot on each target repository's `main` branch.
  Go consumers can track `main` via pseudo-versions
  (`go get github.com/altessa-s/atlas-proto-gen-go@main`); Java tracks
  the `SNAPSHOT` channel.

## Day-to-day: the Two-PR dance

go-atlas itself depends on `atlas-proto-gen-go`. When a `.proto` change
introduces new types, the bindings must reach `atlas-proto-gen-go` **before**
go-atlas can use them — otherwise the main branch breaks. Schema changes
therefore land via **two PRs**:

### PR #1 — schema only

1. Branch in go-atlas. Edit `proto/<service>/v1/*.proto`.
2. Validate locally:
   ```
   make proto-go
   make proto-java
   ```
   If you touched `fieldmasktest`, also run `make proto-testpb` and stage
   the regenerated `proto/gen/fieldmasktest/v1/test.pb.go`.
3. Open PR. [`proto-check.yml`](../.github/workflows/proto-check.yml)
   runs `buf lint` and `buf breaking` against the base branch.
4. Merge. [`proto-publish.yml`](../.github/workflows/proto-publish.yml)
   regenerates and pushes a snapshot to each target repo's `main`.

### PR #2 — consume the new types

Only when go-atlas needs to call the new fields/RPCs.

1. Branch in go-atlas after PR #1's publish workflow finished.
2. Bump the binding:
   ```
   go get github.com/altessa-s/atlas-proto-gen-go@main
   go mod tidy
   ```
3. Update consumer code (handlers, interceptors, helpers).
4. Run the standard local checks:
   ```
   make fmt
   make copyright
   go build ./...
   go vet ./...
   go test ./... -race -count=1
   ```
5. PR → merge.

If a schema change introduces no new go-atlas usage, PR #2 is unnecessary —
external consumers still get the update via PR #1's snapshot.

## Cutting a release

1. Ensure go-atlas `main` is in sync: PR #2 (if any) has merged and
   `make test` is green.
2. Tag go-atlas: `git tag vX.Y.Z && git push origin vX.Y.Z`.
3. `proto-publish.yml` fires on the tag:
   - Regenerates bindings from the tagged commit (filtered to
     `--path scheduler/v1 --path protovalidate/v1`).
   - Pushes to each target repo's `main` (usually a no-op commit — the
     content already matches the latest snapshot).
   - Force-tags each target repo `vX.Y.Z`.
   - For Java, fires `repository_dispatch(event_type: publish-jar)` to
     [`atlas-proto-gen-java`'s publish workflow](https://github.com/altessa-s/atlas-proto-gen-java/blob/main/.github/workflows/publish.yml),
     which runs `gradle publish` against GitHub Packages.
4. Verify:
   - `atlas-proto-gen-go` has the `vX.Y.Z` tag at the matching commit.
   - `atlas-proto-gen-java` has the `vX.Y.Z` tag, and the JAR appears
     under the repo's *Packages* tab.

go-atlas's own `go.mod` at the `vX.Y.Z` tagged commit references
`atlas-proto-gen-go` via a Go pseudo-version. That pseudo-version
resolves to the same commit as `atlas-proto-gen-go vX.Y.Z`, so external
consumers who pin both modules to the same semver tag get a consistent
view. The pseudo-version is intentional — keeping main building with
the Two-PR dance avoids a chicken-and-egg where the tagged commit would
have to reference a tag that doesn't exist yet.

## CI workflows

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| [`proto-check.yml`](../.github/workflows/proto-check.yml) | PR touching `proto/**` | `buf lint` + `buf breaking` against the PR's base branch |
| [`proto-publish.yml`](../.github/workflows/proto-publish.yml) | Push to main (paths `proto/**`), tag `v*`, manual dispatch | Matrix `[go, java]`: regenerate, sync into target repo, push, mirror tag, dispatch Java JAR publish |

The publish workflow mints a short-lived token from the
`altessa-proto-bot` GitHub App for each matrix leg, scoped to a single
target repository. Branch protection on each target's `main` must list
the App as a bypass actor.

## Local generation

| Target | Output | Committed? |
|--------|--------|-----------|
| `make proto-go` | `proto/gen/go/` | No — gitignored, ephemeral mirror of what CI publishes |
| `make proto-java` | `proto/gen/java/` | No |
| `make proto-testpb` | `proto/gen/fieldmasktest/` | Yes — internal test fixture |
| `make proto` | All of the above | — |

### Toolchain prerequisites

`make proto-go` uses local plugins managed via Go's tool dependencies:
[`protoc-gen-go`](https://pkg.go.dev/google.golang.org/protobuf/cmd/protoc-gen-go)
and
[`protoc-gen-go-grpc`](https://pkg.go.dev/google.golang.org/grpc/cmd/protoc-gen-go-grpc).
Both ship via `go install`.

`make proto-java` needs `protoc-gen-grpc-java` on `$PATH`. The Buf
Schema Registry hosts a remote version of this plugin
(`buf.build/grpc/java`) but it returns `403 Forbidden` unless you run
`buf registry login`. We pin the **local** plugin instead — download
the binary from Maven Central and place it on `$PATH`:

```
GRPC_JAVA_VERSION=1.68.1
curl -fsSL -o "$HOME/go/bin/protoc-gen-grpc-java" \
  "https://repo.maven.apache.org/maven2/io/grpc/protoc-gen-grpc-java/${GRPC_JAVA_VERSION}/protoc-gen-grpc-java-${GRPC_JAVA_VERSION}-osx-aarch_64.exe"
chmod +x "$HOME/go/bin/protoc-gen-grpc-java"
```

Substitute the file suffix for your platform (`linux-x86_64`,
`linux-aarch_64`, `osx-x86_64`). CI installs the Linux binary
automatically in `proto-publish.yml`'s Java leg.

## Adding a new schema package

1. Create `proto/<package>/v1/` and add the `.proto` file.
2. Set the standard package options:
   ```
   syntax = "proto3";
   package io.altessa.grpc.<package>.v1;

   option java_package = "io.altessa.grpc.<package>.v1";
   option java_multiple_files = true;
   option go_package = "github.com/altessa-s/atlas-proto-gen-go/<package>/v1;<package>v1";
   ```
3. Append the new path to the publish workflow's filter:
   ```yaml
   buf generate --template buf.gen.${{ matrix.language }}.yaml \
     --path scheduler/v1 \
     --path protovalidate/v1 \
     --path <package>/v1
   ```
   and to the matching `make proto-go` / `make proto-java` targets in
   the [`Makefile`](../Makefile).
4. Extend `.github/scripts/sync-proto.sh` to wipe and copy the new
   subtree in each target repo.
5. Update the Go target's `README.md` (Packages table) and Java's
   build configuration if any new transitive dependencies appear.
6. Land as a normal Two-PR change.

If the new schema is a **test fixture** (like `fieldmasktest`), generate
it locally via `make proto-testpb` and commit the output instead of
adding it to the publish filters.

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `make proto-java` exits with `permission_denied: 403 Forbidden` and `plugin java: signal: killed` | The template references a BSR remote plugin (`buf.build/grpc/java`) and you're not logged into `buf.build`. | Use the local plugin (the repo's `buf.gen.java.yaml` already does this). Install `protoc-gen-grpc-java` from Maven Central — see "Toolchain prerequisites" above. |
| `proto-publish.yml` fails on `Mint GitHub App token` step | The App is not installed on the target repo, or org secrets `PROTO_BOT_APP_ID` / `PROTO_BOT_PRIVATE_KEY` are missing. | Install the App on `atlas-proto-gen-{go,java}` and verify secrets exist at the org or repo level. |
| Workflow pushes but the target repo's main is unchanged | `sync-proto.sh` detected an empty diff — the generated output already matched. | This is expected and intentional. The tag mirror (on tag triggers) still runs. |
| go-atlas main fails to build with `no required module provides package github.com/altessa-s/atlas-proto-gen-go/...` | The Two-PR dance was skipped: consumer code references types from a binding version that doesn't exist yet. | Land PR #1 (schema only) first, wait for `proto-publish.yml`, then bump `go.mod` in a follow-up. |
| `buf breaking` fails on a schema PR | The change is wire-incompatible (renamed field, changed type, etc.). | If intentional and acceptable, annotate with `// buf:lint:ignore` and justify in the PR description; otherwise revise to be additive. |

## References

- [`proto/README.md`](../proto/README.md) — package-level reference for the schema directory
- [Architecture](architecture.md) — overall package layout of go-atlas
- [Buf documentation](https://buf.build/docs/)
- [GitHub Packages — Maven](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-apache-maven-registry)
