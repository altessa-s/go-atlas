# Message Brokers and Queues

- **Status:** Reference. Only section 9 is normative.
- **Audience:** Backend engineers, architects.
- **Version:** 1.0
- **Related:** [Concurrency and Consistency Patterns](concurrency-patterns.md), [Observability](observability.md), [Performance and
  Profiling](performance-profiling.md), [Coordination & Consistency](../coordination.md)

---

## 0. About this guide

This guide covers how message brokers differ from one another and which one to choose for a given task. Read sections 1–2 before design work. Section 3
explains the concepts that cause the most confusion, sections 4–7 cover specific products, and section 8 describes the patterns without which any broker
turns into a source of incidents.

The key difference from choosing a database: **a database fails loudly, a broker fails silently.** A poorly chosen database immediately hits a latency
wall or lacks a query you need, and that is visible. A misconfigured broker runs for years while silently losing one message in three thousand, and the
problem surfaces only when a customer asks why a payment was never credited. For brokers, therefore, a mistake in delivery semantics costs more than a
mistake in product choice.

Second: **the broker is almost never the bottleneck.** The bottlenecks are consumers, retries, and missing idempotency. The choice between NATS and
Kafka affects the architecture far less than the answer to the question "what happens if this message is delivered twice?"

---

## 1. Selection criteria

| Criterion | What to clarify |
|---|---|
| **Delivery semantics** | Is losing a message acceptable? Is a duplicate acceptable? Which is more expensive? |
| **Ordering** | Is global ordering required, per-key ordering, or none at all? |
| **Consumption model** | Each message to one handler (queue) or to all subscribers (bus)? |
| **Retention** | Does a message disappear after processing, or is the log kept and replayable? |
| **Throughput** | Hundreds per second, tens of thousands, millions? |
| **Latency** | Single-digit milliseconds, or are seconds acceptable? |
| **Message size** | Kilobytes or megabytes? Are there attachments? |
| **Number of topics** | Dozens, or hundreds of thousands created dynamically? |
| **Fan-out** | How many independent consumers does one event have, and will new ones appear? |
| **Replay** | Will a new service need to reread the history from the beginning? |
| **Operations** | Who administers it, how is it upgraded, what happens on split-brain, how is a node loss survived? |
| **Transactionality** | Must the message be published atomically with a database write? |

The answer to the last question is almost always "yes", and it is almost always forgotten. See section 8, transactional outbox.

---

## 2. Quick reference

| Task | Primary choice | Alternative |
|---|---|---|
| Events between microservices, integration | NATS JetStream | Kafka, RabbitMQ |
| Background jobs within a single service | PostgreSQL + `SKIP LOCKED` | asynq (Redis), NATS WorkQueue |
| Event log with long retention and replay | Kafka | NATS JetStream with long retention, Pulsar |
| Complex routing, priorities, per-message TTL | RabbitMQ | — |
| Device telemetry, IoT, beacons | MQTT (EMQX, Mosquitto) | NATS with leaf nodes |
| Analytics stream into ClickHouse | NATS JetStream or Kafka | Direct batch insert |
| Real-time notifications without guarantees | NATS Core, Redis Pub/Sub | WebSocket directly |
| Orchestration of long-running business processes, sagas | Temporal | Hand-written sagas on JetStream |
| Deferred execution, scheduling | River / asynq | Broker with delayed queues |
| CDC from the database to the bus | Debezium | MongoDB change streams |
| Queue in AWS with zero operations | SQS + SNS | EventBridge |

---

## 3. Commonly confused concepts

### A queue and a log are not the same thing

**Queue.** A message is taken by one consumer and disappears once acknowledged. The goal is to distribute work among workers. Scaling is
straightforward: add workers and the backlog drains faster. Examples: RabbitMQ classic queues, SQS, JetStream in WorkQueue mode.

**Log.** Messages are appended to an ordered sequence and kept for a configured period regardless of who has read them. Each consumer tracks its own
position and reads at its own pace. A new service can attach and reread everything from the beginning. Examples: Kafka, Pulsar, JetStream in Limits
mode, Redis Streams.

The difference is architectural, not technical. A log lets you add a consumer retroactively and rebuild its state from history. A queue does not:
whatever was processed is gone forever. If you suspect that a year from now a service will need today's events, choose a log.

### Delivery semantics

**At-most-once.** Fire and forget. A message may be lost. Suitable for metrics, telemetry, and live UI updates — cases where the next value arrives
within a second and supersedes the lost one.

**At-least-once.** The message will be delivered, possibly more than once. This is the working mode for 95% of tasks. Duplicates are not a broker bug
but a consequence of the protocol: the consumer processed the message but crashed before sending the ack, so the broker redelivered it to another
consumer after a timeout.

**Exactly-once.** Does not exist end to end across different systems. What is sold under this name is either a guarantee within the boundaries of a
single broker (Kafka transactions: read from a topic, write to a topic, and commit the offset atomically) or deduplication within a window (JetStream
via `Nats-Msg-Id`). As soon as the message leaves the broker — into your database, a third-party API, or a push notification — the guarantee ends.

**The conclusion to accept once and for all:** design for at-least-once and make consumers idempotent. Everything else is an optimization.

### Ordering

Global ordering and horizontal scaling are incompatible. What actually exists:

- **Ordering within a partition or key.** Kafka: messages with the same key land in the same partition and are processed in order. Different keys are
  processed in parallel with no guarantees between them.
- **Ordering within a subject.** JetStream preserves order within a stream; an ordered consumer guarantees sequential reads without gaps.
- **No ordering at all.** SQS Standard, RabbitMQ with multiple consumers, any queue with competing workers.

A separate trap: **a retry breaks ordering even where it is guaranteed.** A message fails and is scheduled for a retry in 30 s; meanwhile, the next ten
messages go through. If ordering is critical, the retry must block the partition, which means head-of-line blocking with all its consequences.

### Push and pull

Push: the broker pushes messages to the consumer, and the rate is controlled via prefetch/QoS. Pull: the consumer requests a batch when ready. Pull
makes backpressure simpler, because a slow consumer simply asks less often. In JetStream, new services use pull consumers; push is considered a legacy
API.

### Ack, nack, DLQ

Acknowledging processing. Three rules that are expensive to violate:

1. **Ack after processing, not before.** Kafka's timer-based offset auto-commit means losing messages on a crash. Disable it.
2. **A poison message must go to the DLQ, not into an infinite retry loop.** A message that always fails will consume the entire consumer if retried
   forever.
3. **Someone must watch the DLQ.** A dead-letter queue without an alert on its depth is silent data loss with extra steps.

---

## 4. NATS: Core and JetStream

This is our primary broker, so it is covered in more detail.

### NATS Core

Plain pub/sub without persistence. A message goes to the subscribers that are online right now; if there are none, it disappears. Latency is a fraction
of a millisecond, throughput reaches millions of messages per second on modest hardware, and the server is about 20 MB and runs as a single binary.

Subjects are hierarchical, with wildcards: `orders.created`, `orders.*.updated`, `devices.>`. This is more convenient than Kafka's flat topic namespace
and lets you subscribe to slices without creating new entities.

It also has built-in request-reply: `nc.Request()` sends a message and waits for a reply on a temporary inbox subject. The result is RPC over the bus
without separate service discovery.

Use Core for: live UI updates, presence, metrics, health signals, distributed cache invalidation — anything where losing a single message changes
nothing.

### JetStream

A persistence layer on top of Core. It provides what teams usually look to Kafka for, but with far simpler operations.

What matters at design time:

**Streams and retention.** A stream collects messages by subject and stores them under one of three policies. `Limits` is a regular log bounded by time,
size, or message count; any number of independent consumers can read it. `WorkQueue` deletes a message once it is acknowledged, giving a classic job
queue. `Interest` keeps a message while there are interested consumers. The policy determines whether you have a log or a queue, and changing it later
is painful.

**Deduplication.** The `Nats-Msg-Id` header plus a deduplication window (two minutes by default, configurable). A producer that retries a publish after
a timeout will not create a duplicate. It is cheap and eliminates a whole class of problems — always use it. The identifier must be unique per event (an
event ID or outbox-record ID) and persisted, so every retry reuses it; a UUID generated once and stored is fine, while an ID derived from the entity
alone suppresses distinct events for that entity.

**Consumers.** A durable pull consumer with `AckExplicit`, `MaxDeliver`, and a configured `BackOff` is the standard setup. When `BackOff` is set it
overrides `AckWait`, so size `BackOff[0]` with ample headroom over the actual processing time; otherwise you get spurious redeliveries under load. A
plain `Nak()` redelivers immediately and bypasses the backoff; use `NakWithDelay` for delayed retries. Messages that reach `MaxDeliver` emit an advisory
on `$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.<stream>.<consumer>` (subscribe to `...MAX_DELIVERIES.>` for all consumers); hook a DLQ-stream transfer
and an alert onto it.

**Clustering.** Raft, with R1, R3, or R5 replicas at the stream level. We do not use R1 in production even for "unimportant" data: losing the node means
losing the stream.

**KV and Object Store.** On top of streams, JetStream provides a key-value store with watch and an object store. KV is convenient for configuration with
change subscriptions and for distributed leader election. It does not replace a full Redis, but it removes an extra dependency where only a little is
needed.

**Limitations to know in advance.** The default maximum message size is 1 MB (it can be raised, but going above 8 MB is not recommended — use a claim
check). There is no ecosystem comparable to Kafka Connect: connectors to external systems are written by hand. There is also no stream processing in the
style of Kafka Streams or Flink.

**Go:** `nats-io/nats.go`, using the new `jetstream` API instead of the deprecated `nc.JetStream()`.

---

## 5. Kafka and its neighbors

### Apache Kafka

A partitioned distributed log. A topic is split into partitions, each partition is replicated, and consumers join groups that divide the partitions
among themselves. Ordering is guaranteed within a partition, and the message key determines the partition.

Strengths: throughput of millions of messages per second, retention for years if needed, log compaction (keep only the latest value per key, which
yields a state snapshot in a topic), transactions for exactly-once within Kafka, and a huge ecosystem — Connect for integrations, Streams and Flink for
stream processing, Schema Registry for contracts.

Choose it when you specifically need a log with long retention and replay, when an event has many consumers that appear over time, when volumes are in
the hundreds of thousands of messages per second, or when you need the Connect ecosystem and stream processing.

Do not choose it for a hundred messages per second between three services. This is the most common mistake in this area: a three-broker cluster,
metrics, rebalances, major-version upgrades, and a person who understands all of it — all for a load that NATS handles on a single node.

Pitfalls:

- **Rebalancing.** With the classic eager protocol, adding or losing a consumer stops processing for the whole group while partitions are reassigned.
  Cooperative sticky assignment limits the pause to the moved partitions, and the incremental consumer protocol of Kafka 4.0+ removes the group-wide
  synchronization barrier; check which one your clients use.
- **The partition count is effectively permanent.** It can be increased but not decreased, and increasing it breaks the key-to-partition mapping and
  therefore ordering.
- **Offset auto-commit.** Enabled by default, it commits on a timer rather than when processing is done. Disable it.
- **`acks=all` plus `min.insync.replicas=2`.** Without these, a "durable" write is not durable.

Modern versions have dropped ZooKeeper in favor of KRaft, which has noticeably simplified operations.

**Go:** `twmb/franz-go` (the most feature-complete, supports transactions), `segmentio/kafka-go` (simpler), `confluent-kafka-go` (CGO, a wrapper over
librdkafka).

### Redpanda

Kafka-compatible at the protocol level, written in C++ without a JVM, with a thread-per-core architecture and no ZooKeeper/KRaft machinery. Latency and
hardware consumption are noticeably better, operations are simpler, and Kafka clients work as is. If the decision "we need Kafka" has already been made,
consider Redpanda before deploying Kafka itself. Check the license: part of the functionality is under the BSL.

### Apache Pulsar

Separates brokers from storage (BookKeeper), which enables fast broker scaling and tiered storage that offloads cold data to S3. Multi-tenancy and
geo-replication are built in, and subscriptions are more flexible than Kafka's: exclusive, shared, failover, key_shared.

Architecturally more interesting than Kafka, but it has more components to operate (brokers, BookKeeper, ZooKeeper or its replacement) and a smaller
community and talent pool. We do not choose it without a specific reason.

---

## 6. RabbitMQ

A classic broker based on the AMQP model: the producer writes to an exchange, the exchange distributes messages to queues according to binding rules,
and consumers read from the queues.

Its main advantage over the others is **routing**. Direct, topic, fanout, and headers exchanges let you express delivery logic declaratively, in broker
configuration rather than in code. It also offers message priorities, per-message TTL, a dead-letter exchange out of the box, and delayed delivery via a
plugin.

Worth knowing:

- **Quorum queues instead of classic mirrored queues.** Classic queue mirroring was removed in RabbitMQ 4.0; replicated queues are built on quorum
  queues (Raft). Quorum queues do not support some classic-queue features — priorities behave differently across versions, so verify against yours.
- **RabbitMQ Streams.** A later, log-like queue type with retention and replay — the answer to Kafka. It works, but the ecosystem around it is modest.
- **Prefetch (QoS).** Unlimited by default, so a single consumer grabs all messages and leaves the others idle. Set it explicitly.
- **A queue is not storage.** RabbitMQ is designed for queues that are mostly empty. Millions of accumulated messages degrade the broker (lazy mode is
  no longer a separate setting since 3.12 — classic queues now generally write messages to disk and keep only a small in-memory buffer).

Choose it when you need complex routing or priorities. In other cases, JetStream is simpler.

**Go:** `rabbitmq/amqp091-go`.

---

## 7. Other options

### Redis Streams

`XADD` to write, consumer groups with `XREADGROUP`, acknowledgment via `XACK`, stuck messages reclaimed with `XAUTOCLAIM`, trimming via `MAXLEN`. The
result is a perfectly workable persistent queue with at-least-once delivery.

Reasonable when Redis is already deployed, volumes are moderate, and you do not want to run a separate broker. The limitation is the same as for Redis
in general: everything lives in memory, so long retention is not its strength.

**Redis Pub/Sub is not a queue.** No persistence, no acknowledgments: if a subscriber is offline, the message is gone. It is fine for cache invalidation
and live updates and unsuitable for anything that must not be lost. These two are confused regularly.

Lists as a queue work with caveats. `BLPOP` is destructive: a crashed worker takes its message with it. `LMOVE` into a per-worker processing list,
followed by `LREM` as the acknowledgment and a sweeper for stale entries, gives a reliable queue — but you build acks and recovery yourself. If you use
Redis, prefer Streams.

### MQTT: Mosquitto, EMQX, VerneMQ, NanoMQ

A protocol for devices: a compact binary format, operation over unstable links, minimal overhead. For our IoT and BLE components, it is the primary
option, not an alternative.

What the protocol provides: three QoS levels (0 — no guarantee, 1 — at least once, 2 — exactly once via a four-step handshake), retained messages (a new
subscriber immediately receives the last known value for a topic — ideal for sensor state), Last Will and Testament (the broker itself publishes a
message if a device disconnects without saying goodbye), persistent sessions, and topic wildcards `+` and `#`. MQTT 5 added session expiry, shared
subscriptions, and request-response.

Choosing a broker: **Mosquitto** — single node, lightweight, for small deployments and development. **EMQX** — clustered, millions of concurrent
connections, routing rules, bridges to Kafka and databases. **NanoMQ** — for the edge and gateways on low-end hardware.

The typical setup: devices speak MQTT to the broker, the broker bridges the stream into NATS or Kafka, and from there these are ordinary events for the
backend. Do not pull MQTT deeper into the service layer, just as you should not force devices to speak AMQP.

### PostgreSQL as a queue

```sql
SELECT * FROM jobs
WHERE status = 'pending' AND run_at <= now()
ORDER BY run_at
FOR UPDATE SKIP LOCKED
LIMIT 10;
```

An underrated option. It provides something no broker does: **enqueueing a job in the same transaction as the business data change.** No outbox, no dual
write, no drift. In addition, jobs are visible with plain SQL, can be fixed with an `UPDATE`, and the whole history stays in the database.

It handles thousands of jobs per second, which is almost always enough for background jobs within a service. Limitations: it is not a bus between
services, vacuum needs attention under high churn, and latency is determined by the polling interval (or `LISTEN/NOTIFY` if a faster reaction is
needed).

**Go:** `riverqueue/river` — a mature implementation with retries, priorities, scheduling, and job uniqueness. There is no need to write your own on top
of `SKIP LOCKED`.

### Task queues in Go

`asynq` on top of Redis — jobs, scheduling, retries with backoff, a web UI. A good option for background jobs if Postgres is unsuitable for some reason.
`watermill` is not a broker but an abstraction layer over different brokers; it is useful when the transport may change, but you pay for that generality
by losing the specifics of a particular broker.

### Temporal

Not a broker, but a different way to solve the problem. Instead of a saga spread across five consumers, you write the business process as ordinary
linear Go code, and Temporal provides durable execution: a process crash does not lose state, steps are retried according to policy, and compensations
are declared explicitly.

Consider it when you have multi-step processes with compensations, timeouts, and waits for external events, and decomposing them into messages has
become painful. The cost is one more serious component to operate.

### Cloud services

**SQS.** Standard — at-least-once without ordering, practically unlimited throughput. FIFO — ordering within a message group and deduplication, but with
rate limits. The consumer acknowledges by calling `DeleteMessage` after successful processing; until then the visibility timeout hides the message, and
it reappears if not deleted. A declaratively configured DLQ, messages up to 1 MiB. Zero operations. AWS only.

**SNS.** Fan-out over SQS, HTTP, and Lambda. SQS and SNS together implement the "one event — many independent queues" pattern.

**Google Pub/Sub.** Autoscaling, global, at-least-once, optional ordering by key.

**Azure Service Bus.** Closer to RabbitMQ in capabilities: sessions, transactions, deferred delivery, DLQ.

### What is not a broker

**ZeroMQ, nanomsg** — socket libraries with messaging patterns, without a server and without persistence. Useful for inter-process communication, but
not a broker replacement.

**Debezium** — CDC: reads the Postgres WAL or the MongoDB oplog and publishes changes as events. An integration tool for legacy systems and an
alternative to the outbox when the application cannot be changed. Usually placed in front of Kafka.

**gRPC streams, WebSocket** — transports, not brokers. They have no persistence, retries, or fan-out.

---

## 8. Essential patterns

### Transactional outbox

A problem everyone has and notices late. A service writes an order to the database and crashes before publishing the event. Or it publishes the event
and crashes before committing the transaction. Atomically writing to the database and sending to the broker is impossible — these are two different
systems.

The solution: the event is written to an `outbox` table or collection **in the same transaction** as the business data. A separate process (a poller or
CDC) reads the outbox, publishes to the broker, and marks entries as sent. Publication is at-least-once; duplicates are removed by broker-side
deduplication or consumer idempotency.

In MongoDB, a change stream on the outbox collection reduces delivery latency: the event goes to the broker at the moment the transaction commits rather
than on the next poller tick. It requires a replica set or a sharded cluster — a standalone `mongod` has no oplog, and the stream will not open. The
stream **cannot** replace the poller, however: retry-backoff expiry and stale-lock release are time-driven, produce no insert, and are invisible to the
stream. The change stream therefore runs on top of the poller, not instead of it — the stream shortens the typical case, and the poller remains the
guarantee. In Postgres, the poller role is played by a regular `SKIP LOCKED` query on the table.

The counterpart is the **inbox**: the consumer writes the identifier of the processed message into a table with a unique index, in the same transaction
as the processing result. A redelivery hits the conflict and is discarded. This is the simplest way to achieve idempotency when the operation itself is
inherently non-idempotent.

### Idempotency

The review formulation: **a handler must produce the same result when the same message is redelivered.** Approaches, in order of preference:

1. The operation is idempotent by itself (`SET status = 'paid'` instead of `INCREMENT attempts`).
2. A unique index on the business key; a duplicate is caught by an insert conflict.
3. An inbox table with processed message identifiers and a TTL.
4. An "already done?" check before acting — the weakest option, since the race remains.

### Retry with exponential backoff and jitter

A retry without backoff turns a transient third-party API failure into a self-inflicted DDoS. A retry without jitter synchronizes all consumers and
creates spikes. A retry without an attempt limit turns one broken message into a processing halt.

A working configuration: 5–7 attempts, backoff from one second to several minutes, jitter, and after exhaustion — DLQ and an alert.

Separately: **distinguish transient errors from permanent ones.** A database timeout — retry. Invalid JSON — straight to the DLQ; a thousand retries
will not fix it.

### Claim check

Do not put messages larger than a megabyte into the broker. The file goes to MinIO, and the message carries a reference and a checksum. This applies to
attachments, large exports, and the base64 images that periodically try to ride along in the payload.

### Head-of-line blocking

One slow or failing message blocks the entire partition behind it. Mitigate this with separate retry topics at different delay levels: if processing
fails immediately, move the message to `retry.30s`, then to `retry.5m`, then to the DLQ. The main stream keeps flowing.

### Saga

A distributed transaction as a sequence of local steps with compensations. Choreography (each service listens to events and reacts) is simpler at the
start, but six months later nobody can draw the full process path. Orchestration (a coordinator explicitly invokes the steps) requires a separate
component, but the process is visible in one place. For three or four steps, use choreography; beyond that, an orchestrator or Temporal.

### Event versioning

An event is a public contract between services, and it outlives the code that produced it. Rules: add new fields only as optional; never remove or
rename existing ones; version the event type in the subject name or in a header; keep the schema in one place and review it separately from the code. An
event in a log with a one-year retention must still be readable a year later.

---

## 9. Default stack

This is the only normative section.

| Task | What we use |
|---|---|
| Events between microservices, sagas, integration | NATS JetStream |
| Background jobs within a service | PostgreSQL + `SKIP LOCKED` (River) |
| Live updates, presence, cache invalidation | NATS Core |
| Telemetry from devices and beacons | MQTT bridged into NATS |
| Event stream into ClickHouse | NATS JetStream, with a consumer that batches inserts |
| Configuration with change subscriptions | JetStream KV |

**Rules for new services.**

1. Events are published only through the outbox. A direct `Publish` from a business transaction does not pass review.
2. Every consumer is idempotent. Review checks this with an explicit question: what happens on redelivery?
3. Every durable consumer has `MaxDeliver`, `BackOff`, and a DLQ route configured.
4. Alerts are configured on DLQ depth and consumer lag. A consumer without a lag alert is considered unconfigured.
5. Subjects follow the `<domain>.<entity>.<event>` scheme, for example `billing.invoice.paid`. On an incompatible change, add a version:
   `billing.invoice.paid.v2`.
6. Kafka is introduced only via an RFC that justifies why JetStream is unsuitable.

---

## 10. Anti-patterns

1. **The broker as a database.** "Kafka stores everything, why do we need a database?" ends with having to replay an entire topic to answer a simple
   question.
2. **A queue where a synchronous response is needed.** If the caller is waiting for the result, it is RPC. Asynchrony adds complexity and gains nothing.
3. **At-least-once without idempotency.** Works only until the first network failure, and the consequences show up in financial reports a month later.
4. **Publishing without an outbox.** The gap between commit and send always exists; the only question is when it will bite.
5. **Assuming ordering where there is none.** Especially after adding a second consumer to the group.
6. **Offset auto-commit.** Messages are lost silently and without a trace.
7. **Infinite retries.** One broken message halts processing of the partition forever.
8. **A DLQ nobody watches.** It is not protection; it is deferred data loss.
9. **Megabytes in the payload.** Use a claim check.
10. **One topic for everything.** Every consumer receives the whole stream and filters in code. The opposite extreme — a topic per message type with a
    dozen subscribers each — is also bad.
11. **Kafka for a hundred messages per second.** The operational cost will never pay off.
12. **Redis Pub/Sub for data that must not be lost.** It has no persistence, and that cannot be configured.
13. **No lag monitoring.** A consumer that is two hours behind looks like a working consumer from the outside.

---

## 11. Summary table

| Broker | Model | Persistence | Ordering | Throughput | Replay | Operational complexity |
|---|---|---|---|---|---|---|
| NATS Core | Pub/Sub | No | No | Millions/s | No | Minimal |
| NATS JetStream | Log and queue | Yes | Within a stream | Hundreds of thousands/s | Yes | Low |
| Kafka | Log | Yes | Within a partition | Millions/s | Yes | High |
| Redpanda | Log (Kafka API) | Yes | Within a partition | Millions/s | Yes | Medium |
| Pulsar | Log and queue | Yes | Within a partition | Millions/s | Yes | High |
| RabbitMQ | Queue and routing | Yes | Within a queue | Tens of thousands/s | Streams only | Medium |
| Redis Streams | Log | In memory | Within a stream | Hundreds of thousands/s | Up to MAXLEN | Low |
| Redis Pub/Sub | Pub/Sub | No | No | High | No | Low |
| MQTT (EMQX) | Pub/Sub with QoS | Optional | Within a topic at QoS 1–2 | Hundreds of thousands/s | Retained messages | Medium |
| PostgreSQL SKIP LOCKED | Queue | Yes | By `ORDER BY` | Thousands/s | Yes, it is a table | None (the database already exists) |
| SQS | Queue | Yes | FIFO only | High | No | None (SaaS) |

---

## 12. Pre-adoption checklist

- [ ] Delivery semantics are defined for each message type
- [ ] For each consumer, the behavior on redelivery is documented
- [ ] Ordering requirements are stated explicitly (or it is explicitly stated that ordering is not required)
- [ ] An outbox is designed for publishing from a transaction
- [ ] `MaxDeliver`, backoff, and a DLQ route are configured
- [ ] The handler code distinguishes transient errors from permanent ones
- [ ] Alerts exist for consumer lag, DLQ depth, and redelivery rate
- [ ] Message size is estimated; a claim check is applied for large payloads
- [ ] A subject naming scheme and an event versioning strategy are defined
- [ ] Behavior when the broker is unavailable is documented: buffering, rejection, degradation
- [ ] The maturity of the Go client has been verified
- [ ] A component owner is assigned

---

## Appendix: FAQ

**When will we outgrow NATS and need Kafka?** When at least one of three conditions holds: a sustained stream above a hundred thousand messages per
second, a need to keep an event log for months with replay for new consumers, or a need for the Connect ecosystem and stream processing. "We have more
events now" is not a reason on its own.

**Can we use NATS alone, without Postgres queues?** You can, but you should not. A background job within a service, enqueued in the same transaction as
the business data, is simpler and more reliable in the database than through a broker with an outbox. The broker is for inter-service communication.

**Do we need an outbox if the broker has deduplication?** Yes. Deduplication solves repeated publication; the outbox solves publication that never
happened at all. These are different failures.

**What if the broker goes down?** The answer must be built into the service design in advance. Options: reject the caller (if the event is critical),
store it in a local buffer (bbolt, the outbox table) and resend after recovery, or degrade functionality. Silently losing it is not an option, yet that
is exactly what happens by default.

**How many partitions or consumers should we configure?** Start with a number equal to the planned processing parallelism and leave headroom: in Kafka,
partitions are easy to add and impossible to remove. In JetStream, parallelism is controlled by the number of instances on a pull consumer and can be
changed freely.
