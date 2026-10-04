# Performance and Profiling

- **Status:** Reference. Sections 10 and 11 are normative.
- **Audience:** Backend engineers, architects.
- **Version:** 1.0
- **Related:** [Concurrency and Consistency Patterns](concurrency-patterns.md), [Message Brokers and Queues](message-brokers.md),
  [Observability](observability.md)

---

## 0. About this guide

This is the document you open when something already hurts. It is therefore structured not as a reference but as a sequence of steps.

The central thesis, from which everything else follows: **do not optimize without measuring.** Intuition about performance is systematically wrong, and
wrong in a predictable direction — people optimize what they understand, not what is slow. The classic outcome of a week of work is a carefully
accelerated function that accounts for half a percent of the time, while a `COLLSCAN` is alive and well in the neighboring module.

The second thesis is specific to our workload: **in Go microservices the CPU is rarely the bottleneck.** It is almost always waiting. Waiting for the
database, for a neighboring service, for a connection from the pool, for a lock, for the disk. In that situation a CPU profile looks dull and shows
nothing — and that is exactly when people close it and start guessing. The right move is to look at an execution trace or break latency down by stage.

The workflow is always the same:

1. State the goal in numbers. "Slow" is not a goal; `p99 < 200 ms` is.
2. Reproduce on data that resembles production.
3. Measure and find where the time goes.
4. Change one thing.
5. Measure again and compare.

Step 4 is violated most often. Three changes at once produce a speedup of unknown origin, and a month later nobody remembers which of them can be rolled
back.

---

## 1. Metrics that tell the truth

### The average is useless

Average latency hides exactly what you care about. A service with a 40 ms mean response time can have a `p99` of three seconds, and those three seconds
are seen by every hundredth user — and with ten requests per page, by every tenth.

Look at percentiles: `p50`, `p95`, `p99`, and `p99.9` when tails matter. Compare instances and time periods by percentiles as well, not by averages.

A caveat on how they are computed: percentiles cannot be averaged. The mean of the `p99` values of ten pods is not the system's `p99`. If your
monitoring system does this, you are looking at a number that means nothing.

### RED and USE

For a service, use **RED**: Rate (requests per second), Errors (error ratio), Duration (latency distribution). Three graphs per service; anything else
as circumstances require.

For a resource, use **USE**: Utilization (how busy it is), Saturation (depth of the wait queue), Errors. Utilization without saturation is misleading: a
disk at 70% utilization with a queue of forty operations performs worse than one at 95% with no queue.

### Tail latency multiplies

A non-obvious effect that hits microservice architectures. If handling a request requires ten parallel calls and each has a `p99` of 100 ms, the
probability that at least one of them lands in its tail is about 10%. In other words, the `p99` of a single call turns into roughly the `p90` of the
composite request.

Two practical consequences follow. Reducing the number of calls in the chain is worth more than speeding up each one. And a dependency's tail is your
typical case, not a rare one.

### Little's law

`L = λ × W`: the number of requests in the system equals the arrival rate multiplied by the processing time. Useful as a back-of-the-envelope estimate:
at 500 rps and 200 ms of processing, about 100 requests are in flight at any moment. If the database connection pool holds 20, you already know where
the queue is.

### Coordinated omission

A load-testing trap. A tool that sends the next request only after receiving the response to the previous one (a closed model) simply stops loading the
system during a stall and never records how bad things got. The result is pretty graphs for a service that has actually degraded.

A load test needs an open model: requests arrive at a fixed rate regardless of whether the system is responding. `k6` and `vegeta` support this, but it
has to be configured explicitly.

---

## 2. pprof

The primary tool. It takes three lines to enable and gives you more than everything else combined.

### Enabling it

```go
import _ "net/http/pprof"

// on a separate internal port, never a public one
go func() {
    log.Println(http.ListenAndServe("127.0.0.1:6060", nil))
}()
```

The import registers handlers on `http.DefaultServeMux`. **Do not expose them on a public port**: a profile reveals memory contents, function names, and
code structure, and `/debug/pprof/profile` also costs CPU time under load. Use an internal port reachable only from inside the cluster.

### Profile types

| Profile | What it shows | When to capture |
|---|---|---|
| `profile` | Where CPU time is spent (30 s sampling) | High CPU usage |
| `heap` | Live objects in memory right now | Growing RSS, suspected leak |
| `allocs` | All allocations since process start | GC pressure, lots of garbage |
| `goroutine` | Stacks of all goroutines | Suspected goroutine leak, hang |
| `block` | Where goroutines wait on synchronization | Latency while the CPU is idle |
| `mutex` | Mutex contention | Poor scaling across cores |
| `threadcreate` | OS thread creation | Rarely; when blocking calls are suspected |

`block` and `mutex` are disabled by default and must be enabled explicitly:

```go
runtime.SetBlockProfileRate(10_000)     // on average, sample one blocking event per 10,000 ns spent blocked
runtime.SetMutexProfileFraction(100)    // sample 1 in 100 events
```

Their overhead is noticeable, so in production enable them for the duration of an investigation, not permanently. Use values above zero, but not one:
`SetMutexProfileFraction(1)` becomes a bottleneck itself under load.

### Viewing profiles

```bash
go tool pprof -http=:8080 http://localhost:6060/debug/pprof/profile?seconds=30
```

This opens a web UI with a flame graph, a call graph, and a listing. Useful interactive commands: `top` (most expensive functions), `list FuncName`
(line-by-line breakdown), `peek` (callers and callees).

**An important distinction in the heap profile.** `inuse_space` is what is occupied right now — look for leaks here. `alloc_space` is the total
allocated over the process lifetime — look for GC pressure here. An object created a million times that dies immediately is invisible in `inuse` but
still loads the GC.

### Comparing profiles

The most underrated mode. Capture a profile before and after a change and look at the difference:

```bash
go tool pprof -http=:8080 -base before.pprof after.pprof
```

This is the only way to confirm that you sped up what you intended rather than shifting the load elsewhere.

### Continuous profiling

A profile captured after an incident shows the system at rest. To have a profile from the moment of the problem, you need continuous collection:
Pyroscope (now part of Grafana) or Parca. The overhead is around one percent, and it pays for itself at the first postmortem, when instead of "please
reproduce it" you can open the flame graph for the exact minute.

---

## 3. Execution tracing

When pprof shows the CPU idling while the service responds slowly, you need `runtime/trace`. It shows not "where computation happened" but "where time
was spent waiting": scheduler activity, GC pauses, blocking on synchronization, network waits, and goroutine state transitions.

```bash
curl -o trace.out 'http://localhost:6060/debug/pprof/trace?seconds=5'
go tool trace trace.out
```

Five seconds is already a lot: the file grows fast and the overhead during collection is significant. Collect in short windows.

What to look for: goroutines waiting while processors are free (meaning they are blocked on an external system or a lock), long GC pauses, and scheduler
latency between "runnable" and "running" (meaning `GOMAXPROCS` is too low or the CPU is taken by neighbors).

To annotate your own code, use regions and tasks; they appear on the timeline:

```go
ctx, task := trace.NewTask(ctx, "processOrder")
defer task.End()

region := trace.StartRegion(ctx, "validate")
// ...
region.End()
```

---

## 4. Benchmarks

### How to write them

```go
func BenchmarkParse(b *testing.B) {
    data := loadTestData() // setup runs once and is excluded from the measurement
    b.ReportAllocs()

    for b.Loop() {
        Parse(data)
    }
}
```

Use `for b.Loop()` (Go 1.24+) rather than the legacy `for i := 0; i < b.N; i++` loop: setup before the loop is excluded from timing automatically, and
the compiler cannot eliminate the call as dead code, so neither `b.ResetTimer()` nor a package-level `sink` is needed. Always call `b.ReportAllocs()`:
allocations per operation are often more informative than nanoseconds. Use `b.RunParallel` when behavior under contention matters.

### A single run means nothing

Run-to-run variance on an ordinary machine easily reaches tens of percent. Comparisons must be statistical:

```bash
go test -bench=. -count=10 -benchmem > old.txt
# make the change
go test -bench=. -count=10 -benchmem > new.txt
benchstat old.txt new.txt
```

`benchstat` reports the median and the variance and — most importantly — tells you whether the difference is significant. Without it, "8% faster" may
turn out to be noise.

### What a benchmark will not tell you

A microbenchmark runs under ideal conditions: the cache is warm, the data fits in L2, there is no contention, and the GC stays out of the way. A
function that wins 30% in a benchmark may gain nothing in a real service, because all the time there is spent waiting on the network. A benchmark
answers "which of two implementations is faster", not "is this worth touching".

---

## 5. Memory and the garbage collector

### How the Go GC works

Concurrent mark-and-sweep, non-compacting, with very short pauses (usually a fraction of a millisecond). The problem in a modern Go service is almost
never pause length but **collection frequency and the amount of marking work**. Many live objects plus a lot of garbage mean the GC runs constantly
alongside your code and eats CPU.

### GOGC and GOMEMLIMIT

`GOGC` (default 100) sets how much the heap may grow relative to live data before the next collection. One hundred means doubling.

`GOMEMLIMIT` is a soft limit on the memory managed by the Go runtime (heap, goroutine stacks, runtime metadata) — not on total process memory. It
arrived in Go 1.19 and solves the main container problem: previously, a service under a cgroup memory limit was killed by the OOM killer because the GC
did not know about the limit and kept growing.

A working configuration for containers: set `GOMEMLIMIT` to roughly 80–90% of the container memory limit. This leaves headroom for memory the runtime
does not manage (cgo allocations, mmapped files, other processes in the container). Being a soft limit, it reduces the risk of OOM but does not
guarantee against it.

A separate technique for latency-sensitive services: `GOGC=off` combined with `GOMEMLIMIT`. The GC then never triggers on heap growth and runs only as
the limit approaches. This works well when the live data size is stable and poorly when it fluctuates.

### GOMAXPROCS in containers

A historical trap that cost many teams weeks of investigation. The Go runtime read the host's CPU count rather than the cgroup limit: a pod limited to
one CPU on a 64-core machine ran 64 OS threads and suffered huge scheduler delays and throttling.

Since Go 1.25 the runtime accounts for cgroup limits on its own. Older versions still need `uber-go/automaxprocs` — a single blank import. Check the Go
version in every service's `go.mod`: if it is older, the dependency is mandatory.

### Escape analysis

The compiler decides whether an object lives on the stack or escapes to the heap. The stack is free; the heap loads the GC. To see its decisions:

```bash
go build -gcflags='-m -m' ./... 2>&1 | grep 'escapes to heap'
```

Typical reasons for escaping to the heap: returning a pointer to a local variable, passing a value as an interface, a closure capturing a variable, and
a slice whose size is unknown at compile time.

### What actually reduces allocations

- **Preallocating slices and maps** when the size is known: `make([]T, 0, n)`. Growing a slice means reallocation and copying.
- **`strings.Builder`** instead of concatenation in a loop. Concatenation creates a new string on every iteration.
- **Reusing buffers** via `sync.Pool` — but only for large, short-lived, frequently created objects. For small structs the pool costs more than the
  allocation, and pooled objects may be dropped at any time — retention across garbage collections is not guaranteed.
- **Working with byte slices instead of strings** where conversion happens on a hot path.

Before doing any of this, look at `alloc_space`. Optimizing allocations in a function responsible for 2% of the garbage is wasted time.

### RSS exceeds the heap, and that is normal

Go does not return freed memory to the operating system immediately. The process `RSS` will be noticeably higher than `inuse_space` in the profile, and
that is not a leak. A leak is `inuse_space` growing over time under steady load.

---

## 6. Profile-guided optimization

An underrated capability: the compiler can use a real profile to optimize. Place a production CPU profile as `default.pgo` in the main package directory
of the binary (or pass `-pgo=path`), and `go build` picks it up automatically — it inlines hot functions more aggressively and lays out code better.

The gain is modest: the [Go documentation](https://go.dev/doc/pgo) reports 2–14% improvements on a set of representative programs; your result depends
on the workload. Not a revolution, but free percentage points for a single file in the repository. Refresh the profile every few releases: a stale one
does no harm, but it helps less.

---

## 7. Where time is actually lost

The list is ordered by how often each item turns out to be the real cause.

### Database

**N+1.** A query for a list, then a query for each item's details in a loop. A hundred items means a hundred-plus round trips. The fix is batching or
aggregation on the database side. Detect it with the metric "queries per incoming call": if it is not constant, you have N+1.

**Missing or unsuitable index.** In Postgres, run `EXPLAIN (ANALYZE, BUFFERS)` and look for a `Seq Scan` where an index was expected, and for a gap
between estimated and actual row counts (a substantial discrepancy may indicate stale statistics or estimation limits). `pg_stat_statements` helps find
the most expensive queries in aggregate rather than per call.

In MongoDB, use `explain("executionStats")`; the key numbers are `totalDocsExamined` versus `nReturned`. A ratio much greater than one means the index
is not selective for the query. A `COLLSCAN` in the plan on a large collection is a finding, not the norm.

**Connection pool.** The most insidious problem on this list, because waiting for a free connection shows up in metrics as a slow query. Configure it
explicitly:

```go
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(25)
db.SetConnMaxLifetime(5 * time.Minute)
```

`MaxIdleConns` lower than `MaxOpenConns` means connections are constantly opened and closed under load. `ConnMaxLifetime` ensures connections are
periodically redistributed across replicas rather than living forever. And count the total: twenty pods with 25 connections each is 500 connections to
the database; with `max_connections` at 200 you are already over the limit. Hence PgBouncer.

### Network and serialization

**The default HTTP client.** `http.DefaultTransport` has `MaxIdleConnsPerHost` set to two. Two. With a hundred parallel requests to one host, the
remaining connections are reopened every time, with a full TLS handshake. Always create your own `Transport` with sensible values.

Plus a rule: the response body must be not only closed but also drained, otherwise the connection does not return to the pool.

**gRPC.** Connections are reused — never create a `ClientConn` per request. For a stream of similar calls, streaming is cheaper than N unary calls.
Remember the default 4 MB limit on received message size.

**JSON.** `encoding/json` relies on reflection and is noticeable on a hot path. Options: switch to protobuf for internal communication, use code
generation (`easyjson`), or a faster implementation (`sonic`). Measure before switching: JSON becomes a bottleneck only at large volumes or very high
rps.

**Redis.** A hundred sequential `GET`s are a hundred round trips. Pipelining or `MGET` turns them into one. With a one-millisecond network delay, the
difference is a hundredfold.

### Your own code

**Logging on a hot path.** Synchronous writes, formatting, and allocations for every field. A debug log inside a processing loop can double the time.
Check the level before building the message, and use sampling for high-frequency events.

**Compiling regular expressions in a loop.** `regexp.MustCompile` belongs in a package-level variable. Always.

**Mutex contention.** Shows up as a failure to scale when adding cores. Fix it by shrinking the critical section, sharding by key (an array of mutexes
indexed by hash), or replacing the mutex with atomic operations. Diagnose it with the `mutex` profile.

**Goroutine leaks.** A goroutine stuck on a channel or on a network call without a timeout lives forever and holds on to its stack. A `goroutine`
profile whose count grows under steady load is a direct sign. The cause is almost always the same: missing context cancellation.

---

## 8. Load testing

Tools: `k6` for HTTP and scenarios, `vegeta` for simple constant rps, `ghz` for gRPC.

What most often spoils the results:

- **An empty database.** A query against a table of a thousand rows fits in cache and shows nothing. Data must be comparable to production in both
  volume and distribution.
- **No warm-up.** The first seconds measure cold caches, connection establishment, and runtime warm-up (heap growth, pool fill-up). Discard the initial
  interval.
- **A run that is too short.** GC, connection rotation, and state accumulation manifest over minutes, not seconds.
- **Uniform load.** The same request with the same parameters warms the cache to an unrealistic state. You need a key distribution that resembles the
  real one.
- **The load generator as the bottleneck.** Verify that you have not hit its limits instead of the system's.

The goal of a test is to find the point after which latency grows nonlinearly and to understand what limits it. A figure like "handles 3000 rps" without
percentiles and without naming the limiting factor is useless.

---

## 9. Anti-patterns

1. **Optimizing without a profile.** The most expensive mistake on this list in terms of time wasted.
2. **Several changes in one pass.** The effect cannot be separated or rolled back.
3. **Optimizing code that is not on the hot path.** A profile reveals this in a minute.
4. **A microbenchmark as proof.** It measures ideal conditions that do not exist in the service.
5. **A single benchmark run instead of `benchstat`.** Variance easily outweighs the effect.
6. **`pprof` on a public port.** Leaks internals and opens a load vector.
7. **An unconfigured `http.DefaultTransport`.** Two idle connections per host.
8. **No `GOMEMLIMIT` in a container.** OOM instead of garbage collection.
9. **`GOMAXPROCS` equal to the host core count on old Go versions.** Throttling and scheduler delays.
10. **Logging inside a processing loop.** Quietly doubles the time.
11. **A default connection pool.** Pool waits masquerade as a slow database.
12. **`sync.Pool` for small objects.** More expensive than a plain allocation.
13. **A load test on an empty database.** Measures the cache, not the system.
14. **A closed load model.** Hides degradation precisely when it occurs.
15. **Caching instead of fixing the query.** Hides the problem until the cache misses.

---

## 10. Defaults

| Task | Decision |
|---|---|
| Production profiling | `net/http/pprof` on an internal port, reachable only from inside the cluster |
| Continuous profiling | Pyroscope |
| Before/after comparison | `go tool pprof -base` |
| Benchmarks | `-count=10` plus `benchstat`; `b.ReportAllocs()` is mandatory |
| Memory limit | `GOMEMLIMIT` ≈ 85% of the container memory budget available to the Go process |
| CPU count | Automatic on Go 1.25+; `automaxprocs` on older versions |
| Build optimization | `default.pgo` in the main package directory, refreshed every few releases |
| Database connection pool | Explicit `MaxOpenConns`, `MaxIdleConns`, `ConnMaxLifetime`; totals computed across all pods |
| HTTP client | A dedicated `Transport`; `DefaultTransport` is not used |
| Load testing | `k6` with an open model on production-scale data |
| Latency metrics | Percentiles and histograms; averages are not shown on dashboards |

---

## 11. Investigation checklist

- [ ] The goal is stated as a number (which percentile of which metric, down to what value)
- [ ] The problem reproduces on data comparable to production
- [ ] A CPU profile has been captured; if the CPU is idle, an execution trace has been captured
- [ ] Database queries per incoming call have been checked (no N+1)
- [ ] Plans of the most frequent queries have been checked
- [ ] Connection pool waits have been ruled out
- [ ] The `goroutine` profile has been checked for leaks
- [ ] If locking is suspected, the `block` and `mutex` profiles have been enabled
- [ ] Only one change has been made
- [ ] The result is confirmed by a profile comparison or `benchstat`
- [ ] The load has been verified not to have shifted elsewhere
- [ ] `block` and `mutex` profiling has been turned back off

---

## Appendix: FAQ

**Where do I start if "everything is slow"?** With a breakdown of latency by stage, not with a profile. How much time went to processing in the service,
how much to each dependency, how much to waiting for the pool. Nine times out of ten that is enough to find the answer, and the profile is no longer
needed.

**Should `encoding/json` be replaced?** Only if the profile shows it in the top three. Replacement yields a real gain at high rps with large structures;
in every other case it is an extra dependency with nonstandard behavior in edge cases.

**Does `sync.Pool` help?** Sometimes a lot, sometimes it hurts. The rule: large, short-lived, frequently created buffers — yes. Small structs — no; the
pool's overhead will exceed the savings. Verify only with a benchmark using `ReportAllocs`.

**What if RSS grows but `inuse_space` does not?** That is not a heap leak. Look at memory outside the profiled heap: goroutine stacks (the `goroutine`
profile), CGO memory, memory-mapped files, and network library buffers. And remember that Go does not return memory to the OS right away.

**Cache or query optimization?** The query first. A cache in front of a slow query masks the problem and leaves it waiting for the moment the cache
misses — and it will miss at the worst time: on a cold start after a rollout or during a burst of unique keys.

**When should we rewrite in another language?** For performance reasons, practically never. In our Go services the CPU is not the limiting factor, and a
rewrite will not speed up waiting for the database. That decision is made on other grounds.
