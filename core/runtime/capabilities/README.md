# capabilities

```go
import "github.com/altessa-s/go-atlas/core/runtime/capabilities"
```

A minimal, cgo-free Go wrapper around the Linux capabilities subsystem
(`capset(2)`, `capget(2)`, `prctl(2)` with `PR_CAPBSET_*` /
`PR_CAP_AMBIENT_*`). It lets a process drop capabilities at startup
and confine itself to a strict allowlist without pulling in libcap.

No root required to drop caps — any unprivileged process can reduce
its own set. Raising caps requires them to already be present in the
bounding ceiling, which is the standard Linux rule and not something
this package works around.

## API

| Symbol | Purpose |
|---|---|
| `Cap` | Strongly-typed capability bit (one constant per kernel `CAP_*`) |
| `Sets` | Snapshot of all five capability sets (effective/permitted/inheritable/bounding/ambient) |
| `Get()` | Read the current snapshot from the calling thread |
| `DropAll()` | Drop every capability everywhere — the "nuclear option" |
| `DropAllExcept(keep ...Cap)` | Drop every capability except those listed |
| `Apply(s Sets)` | Install an exact `Sets` snapshot atomically |
| `BoundingDrop(c Cap)` | Remove `c` from the bounding set (irreversible) |
| `AmbientClearAll()` | Clear the ambient set in one call |
| `AmbientRaise(c Cap)` | Add `c` to the ambient set |
| `AmbientLower(c Cap)` | Remove `c` from the ambient set |
| `ParseName(string) (Cap, error)` | Resolve a canonical `"CAP_*"` string (case-insensitive) |
| `(Cap).String()` | Canonical `"CAP_*"` name |
| `ErrUnsupported` | Platform doesn't support Linux capabilities (non-Linux) |
| `ErrFailed` | The capset/capget/prctl syscall failed (sentinel wrapping the kernel errno via `%w`) |
| `ErrInvalidOption` | Operator supplied a bad capability name or an impossible set |

## Quick start

### Nuclear option — drop everything

```go
package main

import (
    "log"

    "github.com/altessa-s/go-atlas/core/runtime/capabilities"
)

func main() {
    if err := capabilities.DropAll(); err != nil {
        log.Fatalf("capabilities: %v", err)
    }
    run()
}
```

### Bind a privileged port then drop everything else

```go
// Bind first, while CAP_NET_BIND_SERVICE is still effective.
ln, err := net.Listen("tcp", ":443")
if err != nil { log.Fatal(err) }

// Drop everything except what we still need.
if err := capabilities.DropAllExcept(capabilities.CAP_NET_BIND_SERVICE); err != nil {
    log.Fatalf("capabilities: %v", err)
}

serve(ln)
```

### Resolve operator config from YAML

```go
var keep []capabilities.Cap
for _, name := range cfg.Sandbox.Capabilities.Keep {
    c, err := capabilities.ParseName(name)
    if err != nil {
        return fmt.Errorf("sandbox.capabilities.keep: %w", err)
    }
    keep = append(keep, c)
}
if err := capabilities.DropAllExcept(keep...); err != nil {
    return err
}
```

## The four-set model (short version)

Linux capabilities are not a single mask but five related sets
tracked per thread. Most operator use cases only care about the first
three:

| Set | Who uses it | How to change it |
|---|---|---|
| Effective | Kernel permission checks at syscall time | `capset(2)` |
| Permitted | Ceiling from which Effective can be raised | `capset(2)` |
| Bounding | Ceiling from which Permitted can be raised | `prctl(PR_CAPBSET_DROP)` — lowering only |
| Inheritable | Passed across non-privileged execve | `capset(2)` |
| Ambient | Preserved across non-privileged execve | `prctl(PR_CAP_AMBIENT_*)` |

If you do not know which set you need, you want effective/permitted/
bounding. `DropAllExcept` handles exactly those three; inheritable and
ambient are cleared (safe default for a long-running service).

## When to use it

- **Long-running services** launched with effective capabilities
  they no longer need after startup (typical: a service started by
  systemd with `AmbientCapabilities=CAP_NET_BIND_SERVICE`, which
  binds :443 and then drops the cap).
- **Plugin hosts** (see `core/plugins/sandbox.go`) where plugins
  loaded via `dlopen` would otherwise inherit the host's caps.
- **CLI tools** running with file capabilities that want to drop
  them before invoking untrusted subcommands.
- **As one layer in the standard go-atlas hardening sequence:**
  `nonewprivs` → `rlimits` → `capabilities` → `seccomp` → `landlock`.

## Caveats

- **Linux only.** On non-Linux platforms every function returns
  `ErrUnsupported` so cross-platform code can import the package
  without build tags.
- **Per-thread scope.** `capset(2)` mutates only the calling thread.
  The package pins the goroutine to its OS thread around every
  mutating call (`runtime.LockOSThread`), but for the change to
  cover the whole process you must apply **before** spawning any
  goroutines. The plugin sandbox does this correctly; if you call
  `DropAll` from a post-startup code path, expect the change to
  land on one thread only.
- **Irreversible ceilings.** Bits dropped from the bounding set
  cannot be raised by this thread or any descendant. Once you call
  `BoundingDrop` or `DropAllExcept`, you cannot take it back.
- **Raising is bounded.** `Apply` rejects any
  effective/permitted/inheritable bit that exceeds the current
  bounding set with `ErrInvalidOption`. This is not a package
  choice — the kernel enforces it with `EPERM`, but we fail early
  with a clearer message.
- **Defense in depth, not isolation.** A malicious dependency
  already loaded in-process can still read every byte of memory in
  the host's address space. Capability dropping closes the
  "ambient privileged syscalls" vector only.

## Smoke test (manual)

Because this package's mutating calls are irreversible and
process-wide, we do not unit-test them. For real syscall coverage,
build this 40-line harness and run it on a Linux 5.x+ host:

```go
package main

import (
    "fmt"
    "log"
    "net"

    "github.com/altessa-s/go-atlas/core/runtime/capabilities"
)

func main() {
    before, err := capabilities.Get()
    must(err)
    fmt.Printf("before: %+v\n", before)

    must(capabilities.DropAllExcept(capabilities.CAP_NET_BIND_SERVICE))

    after, err := capabilities.Get()
    must(err)
    fmt.Printf("after:  %+v\n", after)

    ln80, err := net.Listen("tcp", ":80")
    if err != nil {
        log.Fatalf("listen :80: %v (expected success)", err)
    }
    _ = ln80.Close()

    must(capabilities.DropAll())

    if _, err := net.Listen("tcp", ":81"); err == nil {
        log.Fatal("listen :81: got nil, want permission denied")
    } else {
        fmt.Printf("listen :81: %v (expected)\n", err)
    }
}

func must(err error) {
    if err != nil {
        log.Fatal(err)
    }
}
```

Run with `go run smoke.go` (the binary must have
`cap_net_bind_service=p` set, e.g. via
`sudo setcap 'cap_net_bind_service=+ep' ./smoke`).

## See also

- [`capabilities(7)` man page](https://man7.org/linux/man-pages/man7/capabilities.7.html)
- `core/runtime/nonewprivs` — `PR_SET_NO_NEW_PRIVS` primitive, first
  step in the hardening sequence
- `core/runtime/rlimits` — resource caps, second step
- `core/runtime/landlock` — filesystem allowlist, final step
- `core/plugins/sandbox.go` — real-world consumer composing all four
