# nonewprivs

```go
import "github.com/altessa-s/go-atlas/core/runtime/nonewprivs"
```

A one-line wrapper around `prctl(PR_SET_NO_NEW_PRIVS)`. Once set, the
calling process and every binary it subsequently execs cannot gain
privileges via SUID/SGID — the kernel silently drops the ambient
escalation. The bit is irreversible for the lifetime of the process.

## API

| Symbol | Purpose |
|---|---|
| `Set()` | Installs the `NO_NEW_PRIVS` bit; **irreversible** for the process |
| `Enabled()` | Reports whether the bit is currently set |
| `ErrUnsupported` | Platform doesn't support the prctl (non-Linux or Linux < 3.5) |
| `ErrFailed` | The prctl syscall failed (sentinel wrapping the underlying errno via `%w`) |

## Quick start

```go
package main

import (
    "log"

    "github.com/altessa-s/go-atlas/core/runtime/nonewprivs"
)

func main() {
    if err := nonewprivs.Set(); err != nil {
        log.Fatalf("nonewprivs: %v", err)
    }
    // From this point onward, no binary this process execs can gain
    // privileges via SUID/SGID.
    run()
}
```

## When to use it

`PR_SET_NO_NEW_PRIVS` is a cheap and narrow defense-in-depth primitive.
Enable it in:

- **Long-running services** that have no reason to exec setuid binaries.
- **CLI tools that ingest untrusted input** and want to prevent injected
  commands from escalating via `sudo`, `mount`, etc.
- **Plugin hosts** (see `core/plugins/sandbox.go`) where a compromised
  plugin must not be able to escalate.
- **Any process preparing to call Landlock** — `landlock_restrict_self(2)`
  requires it. Note that
  [`core/runtime/landlock`](../landlock/README.md) already calls
  `nonewprivs.Set()` internally as its first step, so you do not need to
  call it manually before invoking `landlock.Apply`.

## Caveats

- **Linux 3.5+ only.** On non-Linux platforms `Set` returns `ErrUnsupported`
  so cross-platform code can import the package without build tags.
- **Process-wide and irreversible.** No "unset" syscall. Every goroutine,
  every child process, every binary reached via exec inherits the bit.
- **Breaks legitimate SUID tools.** After `Set` returns, `sudo`, `su`,
  `mount`, and any other setuid helper will silently fail to gain the
  expected privileges. Only enable in processes that have no legitimate
  need to invoke setuid binaries.
- **Not a sandbox on its own.** This primitive only blocks privilege
  escalation on exec. It does nothing about filesystem access (use
  [`core/runtime/landlock`](../landlock/README.md)) or resource limits
  (use [`core/runtime/rlimits`](../rlimits/README.md)).

## See also

- [Kernel documentation](https://www.kernel.org/doc/html/latest/userspace-api/no_new_privs.html)
- `core/runtime/landlock` — filesystem allowlist that requires
  `NO_NEW_PRIVS` as a prerequisite
- `core/runtime/rlimits` — process resource limits, natural sibling
  primitive for startup-time hardening
- `core/plugins/sandbox.go` — real-world consumer composing all three
