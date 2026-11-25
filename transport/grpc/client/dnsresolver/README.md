# dnsresolver

```go
import "github.com/altessa-s/go-atlas/transport/grpc/client/dnsresolver"
```

Package `dnsresolver` resolves the `dns` targets of gRPC client connections and keeps their address lists current. [`transport/grpc/client`](..)
keeps grpc-go's resolver selection by default to preserve DNS TXT service configs and globally registered builders. Enable the Atlas resolver
with `client.WithDNSResolver()`, or install it directly on a `grpc.NewClient` connection with `grpc.WithResolvers`.

```go
conn, err := grpc.NewClient("inventory.default.svc:9000",
    grpc.WithResolvers(dnsresolver.NewBuilder(dnsresolver.WithRefreshInterval(10*time.Second))),
    grpc.WithTransportCredentials(creds),
)
```

## Why

grpc-go's built-in `dns` resolver resolves once and then only when a subchannel fails, and after a failed or empty lookup it backs off up to
120s without reacting to re-resolution requests. With a headless Kubernetes service, whose DNS name returns one address per ready pod, this has
two consequences: a client does not see pods added by a scale-up until one of its connections breaks, and after every pod was gone for a while
the client can keep failing for a minute or more after the pods are back. This package re-resolves periodically and retries failed lookups
within a few seconds. The [ADR](../../../../docs/adr/0002-grpc-client-dns-resolver.md) records the analysis.

## Behavior

| Situation                    | Behavior                                                                                                             |
|------------------------------|----------------------------------------------------------------------------------------------------------------------|
| Lookups succeed              | The host is resolved again after the refresh interval, shortened at random by up to the refresh jitter; the connection gets a new state when the address set changes |
| `ResolveNow`                 | The next lookup starts no earlier than the minimum interval after the previous one ended; bursts are merged          |
| One or both family lookups fail | Publish usable answers and keep the addresses of each failed family. Report the error and retry with jittered backoff, sooner on `ResolveNow` |
| Both families return no addresses | Submit an empty state and report `no addresses for host`. Retry with jittered backoff |
| Recovery                     | The state is reported at once, also when the addresses are the ones known before the failure                         |
| IP address target            | Resolved once to itself, no polling                                                                                  |
| `Close`                      | Cancels the lookup in flight and the pending timer and returns after the resolver goroutine exits                    |

The resolver queries A and AAAA independently using Go's DNS resolver, with one shared timeout. A timeout, SERVFAIL or other lookup error
keeps the addresses last published for that family. If the combined set would be empty and either family failed, the connection keeps its
previous addresses. Only conclusive negatives from both families (NXDOMAIN or no records) trigger an empty update.

The default round-robin policy removes its subchannels on an empty update, allowing a server that returns on its old address to connect using
a fresh subchannel. Other policies may reject the update and retain their subchannels.

The resolver combines IPv4 then IPv6, preserving each family's answer order and comparing address sets without regard to order. It performs
address lookups only. The connection uses the client's default service config, such as the round-robin, health check and retry settings from
`transport/grpc/client`.

`NewBuilder` never calls `resolver.Register`: the builder serves only the connections it is passed to, and other gRPC users of the process keep
grpc-go's resolver.

## Targets

The forms grpc-go's `dns` resolver accepts: `host:port`, `dns:///host:port`, `dns://server/host:port` (lookups go to the DNS server at `server`,
port 53 unless given), IPv4 and IPv6 literals (bracketed with a port, as in `[2001:db8::1]:9000`). A target without a port uses port 443; an empty
host written as `:port` means `localhost`. Bracketed empty hosts such as `[]` and `[]:9000` are invalid in targets and DNS authorities.

## Options

| Option                  | Default | Description                                                                                       |
|-------------------------|---------|---------------------------------------------------------------------------------------------------|
| `WithRefreshInterval`   | 30s     | Re-resolution interval while lookups succeed                                                     |
| `WithRefreshJitter`     | 0.1     | Largest fraction by which a refresh interval is shortened at random; 0 disables, above 0.5 is lowered to 0.5 |
| `WithMinInterval`       | 2s      | Shortest pause before a lookup `ResolveNow` requests; lowered to the refresh interval if above it |
| `WithLookupTimeout`     | 10s     | Timeout of one lookup                                                                             |
| `WithBackoffBaseDelay`  | 1s      | Delay after the first failed lookup                                                               |
| `WithBackoffMultiplier` | 1.6     | Growth per further failure; below 1 is raised to 1                                                |
| `WithBackoffJitter`     | 0.2     | Randomized fraction of each delay; 0 disables jitter, above 1 is lowered to 1                     |
| `WithBackoffMaxDelay`   | 5s      | Cap of the delay; jitter never exceeds it, at the cap it only shortens the delay                  |
| `WithLogger`            | discard | Logger for state transitions                                                                      |
| `WithMeter`             | no-op   | Meter for the metrics below                                                                       |

The defaults: the refresh interval matches the minimum re-resolution interval of grpc-go's resolver, now applied as a period. The refresh jitter
only shortens the interval, so clients started by one rollout drift apart instead of querying the DNS server in lockstep, while a change is still
seen within the refresh interval. The minimum interval
folds the burst of `ResolveNow` calls one server failure causes (one per failing subchannel) into one lookup, and stays below the backoff cap so a
request still shortens a retry while lookups fail. The lookup timeout covers the default per-query timeout of resolv.conf (5s) with one retry.
The backoff starts like grpc-go's connection backoff (1s, ×1.6, ±20%) but stops at 5s, which bounds how long a client needs to see addresses
that return after an outage. At the cap the jitter range ends at the cap instead of the delay being clipped to it, which would give half of the
retries the same delay and keep clients that reached the cap together in lockstep.

## Headless and ClusterIP services, MaxConnectionAge

With a headless service the resolver sees every pod. A scale-up reaches existing clients within the refresh interval; a pod replacement is
picked up when the failing subchannel asks for re-resolution, at the latest at the next refresh. After a full outage, the resolver can pick
up returning pods within the backoff cap. With the default round-robin policy, pods returning on their old addresses also get fresh
subchannels. Clients rebalance onto new pods without waiting for `MaxConnectionAge` (30m by default in [`config/grpc`](../../../../config/grpc)).

These bounds start when the DNS server answers with the change. CoreDNS caches answers: the Corefile of the Kubernetes documentation and kubeadm
sets `ttl 30` in the `kubernetes` plugin and `cache 30`, so an answer, NXDOMAIN included, can be up to 30s old. A new pod then reaches clients
within about the refresh interval plus 30s, and pods returning after a full outage within about 30s plus the backoff cap. Lower the plugin's `ttl`
to shorten both; refreshing much more often than the cache expires only adds queries.

A ClusterIP service resolves to one virtual IP that never changes, so lookups never fail and the resolver changes nothing: after an outage the
client reconnects according to its connection backoff, which `transport/grpc/client` exposes as `WithConnectBackoff` (grpc-go's 120s cap by
default). Load spreads over the pods only per connection there, which `MaxConnectionAge` keeps renewing.

## Observability

The `grpc_dns_resolver` subsystem aggregates metrics across all targets on the meter.

| Name | Type | Labels | Description |
|------|------|--------|-------------|
| `grpc_dns_resolver_lookups_total` | Counter | `result` | Attempts classified as `ok`, `empty` (both families conclusive) or `error` (either family failed) |
| `grpc_dns_resolver_addresses` | Gauge | — | Sum of addresses last submitted by active polling resolvers. Closing a resolver removes its contribution |

Logs: one warning when lookups start failing (`dns resolution failing`, with the error) and one info line when they recover
(`dns resolution recovered`, with the address count, the failed attempts and the downtime). Successful polls are not logged.
