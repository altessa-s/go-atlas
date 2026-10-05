# Observability

- **Status:** Reference. Sections 10–12 are normative.
- **Audience:** Backend engineers, architects, on-call engineers.
- **Version:** 1.0
- **Related:** [Concurrency and Consistency Patterns](concurrency-patterns.md), [Message Brokers and Queues](message-brokers.md), [Performance and
  Profiling](performance-profiling.md), [Metrics Reference](../metrics.md), [Logging (slog)](../observability/slog.md),
  [Health](../observability/health.md)

---

## 0. About this guide

The other guides in this set keep arriving at the same phrase: "set up an alert", "there must be monitoring", "this is verified by a metric". None of
them says exactly where to look or what justifies waking someone up. This guide closes those gaps, which is why section 6 consists in part of
requirements collected from the other guides.

The difference between monitoring and observability in one sentence: **monitoring answers the questions you thought of in advance; observability lets
you ask a new question at 3 a.m. without shipping a release.** The first is a dashboard with CPU utilization. The second is the ability to ask "show me
every request from this customer in the last ten minutes that took longer than a second, and what inside them was slow".

A word on priority up front, because the rollout order is usually chosen wrong. Start not with tracing, however fashionable it is, but with correlation:
`trace_id` in every log line and end-to-end context propagation across every boundary, including the broker. Without it, the three data sources remain
three unrelated sources, and an investigation turns into matching timestamps by eye.

---

## 1. Signals and what each one answers

| Source | Cost | Question it answers |
|---|---|---|
| **Metrics** | Low; volume does not depend on traffic | Is something wrong? How badly? When did it start? |
| **Traces** | Medium; depends on sampling | Where exactly in the service chain is the problem? |
| **Logs** | High; grows linearly with traffic | What exactly happened to this request? |
| **Profiles** | Low with continuous collection | Why is this piece of code expensive? |

During an incident, work proceeds top-down: a metric fires the alert, a trace shows which service and which step the time is spent in, a log provides
the details of the specific case, and a profile explains why the code is slow. Starting with logs is the slowest path available.

The same ordering defines the budget allocation rule: plenty of metrics, enough traces, and exactly as many logs as people actually read.

---

## 2. Correlation: the prerequisite for everything else

### Identifiers

Every request has a `trace_id` that is created at the system's entry point and propagates to its synchronous descendants; asynchronous descendants
either continue it or start their own traces linked back to it (see below). Each unit of work inside it is a `span_id`. These two fields must be
present:

- in every log line,
- in every trace span,
- in metric exemplars, where supported,
- in the error response to the client (so that support can find the request from a ticket without asking the user what time it happened).

The last item costs next to nothing and saves hours. Return the `trace_id` in the error body or in a response header.

### Propagation across boundaries

The standard is W3C Trace Context, the `traceparent` header. Over HTTP and gRPC it propagates automatically once instrumentation is wired in. The
trouble starts where the boundary is crossed not by a call but by a message.

**The asynchronous boundary is where everyone loses context.** An event goes out to NATS, a consumer picks it up, and a new trace starts that is
unrelated to the original. As a result, the chain "user clicks a button → service writes → event → three consumers → push notification" falls apart into
five independent traces that can only be connected manually by a business identifier.

The fix is to inject the context into message headers on publish and extract it on consume. NATS has headers, Kafka has headers, and this works for
both.

A subtle point of semantics: a parent-child relationship expresses causality, not waiting, so making the consumer span a child of the publish span is
valid for single-message processing. For batch consumption and long fan-out chains, start a new trace on the consumer side and attach a **link** to the
producer's span context: it keeps traces from growing to thousands of spans, and correlation across the distinct trace IDs goes through the links.

### Mandatory log context

Besides `trace_id`, every line carries the service name, build version, environment, and instance identifier. Without the version, it is impossible to
tell whether a spike of errors relates to the latest rollout.

---

## 3. Metrics

### Histograms, not averages

Why averages are useless is covered in [Performance and Profiling](performance-profiling.md); this section is about implementation.

Prometheus-compatible systems offer two types for distributions. A **histogram** counts observations into buckets on the service side; the percentile is
computed at query time and aggregates correctly across instances. A **summary** computes percentiles on the service side, and they **cannot** be
aggregated across instances — the average of the `p99` of ten pods is not the system's `p99`.

Rule: histogram always, summary never. The percentile is computed by a query:

```promql
histogram_quantile(0.99, sum by (le, service) (rate(http_request_duration_seconds_bucket[5m])))
```

Bucket boundaries are chosen to match the expected distribution. The default Prometheus buckets are tuned for web requests and fit poorly for operations
that take seconds or, conversely, microseconds.

### Cardinality is the main danger

Every unique combination of label values creates a separate time series. A metric with a `user_id` label and a million users is a million series, and
that does not just load the metrics store — it kills it.

**Never put into labels:** user, request, order, or session identifiers; email addresses; IP addresses; full URL paths with parameters; error messages;
timestamps.

The test: **a label is acceptable if its set of values is finite, small, and does not grow with traffic.** HTTP method — yes. Route template
`/api/orders/{id}` — yes. A concrete path `/api/orders/7f3a...` — no.

Anything needed per identifier lives in logs and traces, not in metrics. That is the division of labor between the sources.

### Mandatory set for every service

**RED for inbound traffic** — per route or gRPC method: request count, error ratio, duration histogram.

**The same for every outbound dependency** — the database, the cache, every service called. Without it, during a slowdown you cannot tell "we are slow"
from "a neighboring service is slowing us down".

**Connection pool** — in use, idle, and time spent waiting in line for a connection. As [Performance and Profiling](performance-profiling.md) explains,
pool waits masquerade as slow queries, and without this metric they are indistinguishable.

**Go runtime metrics** — goroutine count, heap size, GC frequency and duration, scheduler latency. They are collected by a ready-made collector; there
is almost nothing to configure.

**Broker metrics** — consumer lag, number of unacknowledged messages, redelivery rate, DLQ depth.

**Business metrics** in a separate namespace: payments, sign-ups, push notifications sent, credits. They catch what technical metrics miss: the service
responds with 200s while payments fail.

### Naming

Prometheus convention: unit at the end of the name, `_total` for counters, base SI units.

```
http_requests_total
http_request_duration_seconds
mongo_pool_connections_in_use
outbox_pending_age_seconds
```

Not `latency_ms`, not `requests_count`. Keep it identical across services, otherwise shared dashboards and alerts cannot be built.

---

## 4. Logs

### Structured, always

A string log with interpolation is unfit for search. Go ships `log/slog` in the standard library; no extra dependency is needed.

```go
logger.ErrorContext(ctx, "failed to charge funds",
    slog.String("order_id", orderID),
    slog.String("currency", cur),
    slog.Int64("amount_minor", amt),
    slog.Any("error", err),
)
```

The handler is configured once and injects `trace_id`, `span_id`, service name, and version from the context on its own.

### Levels

The boundaries must be the same in every service; otherwise an alert on the ERROR ratio is useless.

- **ERROR** — requires human attention. If nobody will ever act on the entry, it is not an ERROR.
- **WARN** — an anomaly the system handled on its own: a retry succeeded, a fallback kicked in, a circuit breaker opened.
- **INFO** — significant business events, one or two entries per request. Not an execution trace.
- **DEBUG** — disabled in production. Enabled selectively and temporarily.

The most common level mistake is `ERROR` for an input validation failure. A user sent malformed JSON — that is `INFO` or `WARN`; there is nothing to act
on, and the error ratio on the dashboard is polluted.

### Log an error once

An error is either handled or wrapped and returned up the stack. Logging and returning at the same time turns one problem into five lines at different
levels of the stack.

The place to log is the top boundary: the HTTP handler, the gRPC interceptor, the message handler. That is also where the error becomes a response
status. Below it, only wrap with context and return (`fmt.Errorf("...: %w", err)` or the project's equivalent).

> **In go-atlas.** Packages that already use [`core/errors`](../../core/errors) keep its conventions: wrap with `coreerrs.Wrap` / `Wrapf` rather than
> switching to `fmt.Errorf`.

### What must never appear in logs

Tokens, passwords, API keys, card numbers, verification codes, personal data, full payment request and response bodies, push token contents. In
particular, do not log whole structures that may contain any of these — `slog.Any("request", req)` will one day turn out to be exactly that request.

Implement `LogValuer` on domain types with sensitive fields so that masking is a property of the type rather than of the author's discipline.

### Cost

Logs are the most expensive part of observability because their volume grows linearly with traffic. Hence:

- High-frequency events are sampled: one entry in a hundred plus a counter metric.
- A debug log in a hot loop costs performance, not just money.
- Storage is chosen by budget: ClickHouse and VictoriaLogs are several times cheaper than Elasticsearch at the same volume; search is weaker, but good
  enough for logs.
- Retention by level: errors longer, INFO shorter.

---

## 5. Tracing

### OpenTelemetry and nothing else

The industry standard: vendor-neutral and supported everywhere. We do not use vendor SDKs: switching backends must not mean rewriting instrumentation.

Automatic instrumentation covers transports and clients: `otelhttp`, `otelgrpc` (via `StatsHandler`, not the deprecated interceptors), and wrappers for
the Mongo and Redis drivers. This gives you the skeleton of a trace for free.

Manual spans are needed in business logic — where the transport cannot see the structure of the work:

```go
ctx, span := tracer.Start(ctx, "calculateTipDistribution")
defer span.End()

span.SetAttributes(
    attribute.Int("recipients.count", len(recipients)),
    attribute.String("currency", cur),
)
```

On error, the span must reflect it: `span.RecordError(err)` and `span.SetStatus(codes.Error, ...)`. Otherwise the trace looks successful, and a search
for failed traces will not find it.

Span names are low-cardinality, like metric names. Identifiers go into attributes, not into the name.

### Sampling

Tracing everything is expensive in storage and network. There are two approaches:

**Head-based.** The decision is made at the start of the trace, usually at random with a given probability. Cheap, simple, works without a central
component. The drawback: a failed or slow request is dropped with the same probability as a normal one — so exactly what you need is what gets lost.

**Tail-based.** The decision is made in a collector that buffers spans for a decision window and then applies policies — for example, keep every failed
and every slow trace while sampling successful ones. It works only if all spans of a trace reach the same collector instance (trace-ID-aware routing),
upstream services export everything (no head sampling before it), and the window and memory are sized for your longest traces; spans arriving after the
decision are handled separately. It requires the OTel Collector with the `tail_sampling` processor.

**Our rule:** sample successful fast requests; keep in full every request with an error and every request above the latency threshold. In practice that
means tail-based sampling in the collector. Until you get there — head-based at 5–10% plus forced sampling via a context flag for debugging; keep in
mind that head sampling cannot recover a failed trace it has already discarded.

### Exemplars

A mechanism that links a point on a metric graph to a specific trace. You see a spike in the top bucket of a histogram, click it, and land on the trace
of a request that fell into it. A cheap feature that drastically shortens the path from "something is wrong" to "there it is".

---

## 6. Alerts

### The only rule

**An alert is something that requires human action right now.** Everything else is a dashboard or a report.

The test when creating one: what exactly will the on-call engineer do after being woken up by it? If there is no answer, there should be no alert
either. Alert fatigue is not a metaphor: a team receiving twenty notifications a day stops reacting to all of them, including the real ones.

### Symptoms, not causes

Alert on what the user experiences, not on internal indicators. "Error ratio above 1% for five minutes" — yes. "CPU utilization at 85%" — no: that may
be normal during regular operation, or mean nothing for an idle service.

The exception is **leading indicators of resource exhaustion**, where a reaction is needed before the user notices anything: disk space, certificate
expiry, approaching the connection limit or `GOMEMLIMIT`.

### SLOs and error budgets

The objective is phrased as "99.9% of requests succeed and complete within 300 ms over 30 days". The error budget and its burn rate follow from it.

Burn-rate alerts use two windows: fast (a substantial share of the budget within an hour — page the on-call engineer) and slow (a sustained shortfall
over a day — a ticket for working hours). This is an order of magnitude more precise than a fixed threshold and produces almost no false positives.

### Mandatory minimum

This table collects the requirements that the other guides left without an owner. The second column names where the requirement comes from: a guide
in this set, the baseline every service needs, or domain-specific invariants.

| Alert | Source topic |
|---|---|
| Error-ratio SLO violation (fast and slow burn) | Baseline |
| Latency SLO violation | Baseline |
| Consumer lag growing steadily | [Message brokers](message-brokers.md) |
| DLQ depth above zero | [Message brokers](message-brokers.md) |
| Redelivery rate above normal | [Message brokers](message-brokers.md) |
| Age of the oldest unsent outbox record | [Concurrency](concurrency-patterns.md) |
| Goroutine count growing under stable load | [Performance](performance-profiling.md) |
| Connection pool utilization close to the limit | [Performance](performance-profiling.md) |
| Approaching `GOMEMLIMIT`, OOM restarts | [Performance](performance-profiling.md) |
| Materialized balance diverging from the sum of ledger entries | Domain-specific |
| Ratio of failed monetary operations | Domain-specific |
| Database replication lag | Baseline |
| Certificate expiry, disk space | Baseline |

A consumer without a lag alert is considered not configured — that wording comes from [Message Brokers and Queues](message-brokers.md), and it now has
concrete content.

### Runbook

Every alert links to a runbook: what it means, how to verify it, what to do, whom to call. An alert without a runbook shifts all the work onto the
on-call engineer's memory, and on-call engineers rotate.

---

## 7. Dashboards

**One standard overview dashboard per service**, with the same structure for all: RED for inbound traffic, RED for each dependency, runtime, resources.
Uniformity matters more than completeness — the on-call engineer should not have to learn a new layout for every service.

Distinguish two dashboard types. An **on-call** dashboard answers "is everything fine" from three meters away: few graphs, large, understandable without
context. An **investigation** dashboard is for analysis and can be as detailed as needed.

No dashboard should show average latency. Percentiles only.

---

## 8. Health probes

Three different probes that are constantly confused, with expensive consequences.

**Liveness** answers "is the process alive or hung". A failure means the pod is restarted. **External dependencies must never be checked here.** The
classic failure scenario: liveness calls the database, the database blips for ten seconds, Kubernetes restarts every pod of every service at once, and
instead of a ten-second degradation you get a multi-minute avalanche of cold starts.

**Readiness** answers "is it ready to accept traffic". A failure means removal from load balancing without a restart. This is where checking
dependencies is appropriate: no database connection — don't accept requests, but don't die either.

**Startup** protects slow-starting services from being killed by liveness before they are ready.

Readiness must report failure during graceful shutdown — before the service stops accepting connections. Otherwise the load balancer will still send
requests to the terminating process.

---

## 9. What it costs

Observability can cost more than the infrastructure it observes, and this happens regularly. The main levers:

- **Metrics** are cheap as long as cardinality is under control. A single bad label changes that instantly.
- **Logs** are the main expense. They are reduced by sampling, levels, retention, and choice of storage.
- **Traces** are controlled by sampling and short retention: a week-old trace is almost never needed.
- **Metric downsampling**: per-second resolution for the last day, per-minute for a month, per-hour for a year.

A reasonable retention baseline: metrics for a year with reduced resolution, traces for a week, INFO logs for a week, error logs for a month.

---

## 10. Anti-patterns

1. **An identifier in a metric label.** Cardinality explosion, storage failure.
2. **Summary instead of histogram.** Percentiles cannot be aggregated across instances.
3. **Average latency on a dashboard.** Hides exactly what you are looking for.
4. **Logs without `trace_id`.** The three data sources remain unrelated.
5. **Losing context at the broker boundary.** The chain falls apart into unrelated traces.
6. **Logging an error at every level of the stack.** One problem, five entries.
7. **`ERROR` for a validation failure.** Pollutes the error metric and teaches people to ignore the level.
8. **Sensitive data in logs.** Discovered during an audit, expensive to fix.
9. **An alert without a runbook.** The work shifts onto the on-call engineer's memory.
10. **Alerting on a cause instead of a symptom.** Noise while the system is working.
11. **An alert nobody acts on.** Worse than no alert: it teaches people to ignore the channel.
12. **Checking dependencies in liveness.** Turns a database blip into an avalanche of restarts.
13. **DEBUG enabled in production.** Storage cost and lost performance.
14. **Retaining 100% of traces in the backend without justification.** Expensive without proportional benefit (exporting everything to a tail
    sampler is a different matter).
15. **A vendor SDK instead of OpenTelemetry.** Switching backends means rewriting.

---

## 11. Defaults

| Layer | Decision |
|---|---|
| Metrics | VictoriaMetrics, Prometheus format |
| Logs | `log/slog`, JSON, shipped to ClickHouse or VictoriaLogs |
| Tracing | OpenTelemetry, exported via the OTel Collector |
| Profiles | Pyroscope, continuous collection |
| Visualization and alerting | Grafana |
| Context format | W3C Trace Context, `traceparent` |
| Context over NATS | Injected into message headers, connected via a link |
| Trace sampling | Tail-based: all failed and slow, successful ones selectively |
| Timestamps in logs | RFC 3339 in UTC |
| Probes | Liveness without external dependencies, readiness with them |

---

## 12. New service checklist

- [ ] RED metrics are exported for inbound requests
- [ ] Metrics are exported for every outbound dependency
- [ ] Connection pool and Go runtime metrics are exported
- [ ] No metric label contains an identifier or any other unbounded value
- [ ] Distributions use histograms, not summaries
- [ ] Logs are structured; every line has `trace_id`, service, and version
- [ ] Log levels follow the shared convention
- [ ] An error is logged once, at the top boundary
- [ ] Sensitive fields are masked at the type level
- [ ] `otelhttp` or `otelgrpc` is wired in; spans are added in business logic
- [ ] Errors are recorded on the span and change its status
- [ ] Trace context is propagated through message publishing and consumption
- [ ] An overview dashboard exists, built from the standard template
- [ ] An SLO is defined; burn-rate alerts are configured
- [ ] Alerts from the mandatory minimum that apply to the service are configured
- [ ] Every alert has a runbook
- [ ] Liveness does not call external dependencies
- [ ] Readiness reports failure during graceful shutdown
- [ ] `trace_id` is returned to the client on error

---

## Appendix: FAQ

**Where do we start if there is nothing?** With correlation and RED metrics. `trace_id` in logs and three metrics per service give more than full
tracing without a link to logs. Add tracing as the second step; it pays off in cross-service investigations.

**Do we need traces if logs carry `trace_id`?** Logs show what happened; a trace shows how long it took and what was waiting on what. With one service
you can get by with logs; with a chain of five you cannot — summing durations by hand from timestamps in different services does not work.

**Where do business metrics go — metrics or analytics?** Both, for different purposes. In metrics — coarse and operational, for alerts: "payments
dropped to zero". In ClickHouse — precise and detailed, for reports. Do not try to count money in Prometheus: it stores scrape-interval samples, not
individual events, and counter resets on restarts make exact totals unreliable.

**What do we do with an alert that fires often and is always false?** Remove or rework it the same day. The intermediate state "we know about it and
ignore it" is the most dangerous: the habit of ignoring spreads to the whole channel.

**How much should traces be sampled?** Start with 10% head-based sampling; keeping all failed traces requires tail-based sampling. Then watch storage
volume and how often the trace you need turns out to be missing during an investigation. If it is missing regularly, move to tail-based sampling rather
than raising the percentage.
