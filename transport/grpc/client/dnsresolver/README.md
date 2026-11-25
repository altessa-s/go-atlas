# dnsresolver

```go
import "github.com/altessa-s/go-atlas/transport/grpc/client/dnsresolver"
```

Package `dnsresolver` resolves the `dns` targets of gRPC client connections and keeps their address lists current. [`transport/grpc/client`](..)
installs it by default for targets without a scheme and `dns` targets; any `grpc.NewClient` caller can install it with `grpc.WithResolvers`.

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
| Lookup fails                 | A timeout, SERVFAIL or other error is reported to the connection, which keeps its addresses; the lookup is retried with exponential backoff and jitter under the cap, sooner on `ResolveNow` |
| DNS finds no address         | NXDOMAIN or no records: the connection's addresses are dropped with an empty state, which closes their subchannels, and the error `no addresses for host` is reported; retried like a failed lookup |
| Recovery                     | The state is reported at once, also when the addresses are the ones known before the failure                         |
| IP address target            | Resolved once to itself, no polling                                                                                  |
| `Close`                      | Cancels the lookup in flight and the pending timer and returns after the resolver goroutine exits                    |

A failed lookup says nothing about the servers, so the connection keeps using the addresses it has. An answer that no address exists is what DNS
gives for a headless service without ready pods; it drops the addresses, like Envoy's `STRICT_DNS` clusters and Finagle do. Their subchannels close,
so a server that returns gets a new subchannel and is connected at once, even on its old address (a container restarted in place keeps its pod IP).
Kept, the old subchannel would first wait out the connection backoff its failed attempts built up, up to 120s by default. Until the first state the
error is what RPCs fail with; after a drop the round-robin balancer reports `no children to pick from`.

A lookup sends one query per address family and search domain. When one of them times out or fails while others answer, the whole lookup fails
(`net.Resolver.StrictErrors`): a partial answer would otherwise be reported as a smaller address set and drop the addresses of the failed query.
Go's built-in resolver honors this; the cgo resolver, which Go uses on macOS for example, does not.

Address order never causes an update: a set is compared sorted and deduplicated, and reported in the order DNS returned it. A lookup has a
timeout. SRV and grpclb lookups are not performed. Service configs in DNS TXT records are not looked up either, so the connection always uses
its default service config, such as the one `transport/grpc/client` sets for round-robin balancing, health checking and retries; grpc-go's
resolver would let a `_grpc_config` TXT record replace it.

`NewBuilder` never calls `resolver.Register`: the builder serves only the connections it is passed to, and other gRPC users of the process keep
grpc-go's resolver.

## Targets

The forms grpc-go's `dns` resolver accepts: `host:port`, `dns:///host:port`, `dns://server/host:port` (lookups go to the DNS server at `server`,
port 53 unless given), IPv4 and IPv6 literals (bracketed with a port, as in `[2001:db8::1]:9000`). A target without a port uses port 443; an empty
host before a port means `localhost`. An empty target, `[]`, a target that ends with `:` and a malformed address fail the build with an error.

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
picked up when the failing subchannel asks for re-resolution, at the latest at the next refresh. After a full outage clients reconnect within
the backoff cap of the pods returning to DNS, on new addresses and on their old ones alike. A server's `MaxConnectionAge` (30m by default in
[`config/grpc`](../../../../config/grpc)) is not needed to rebalance clients onto new pods.

These bounds start when the DNS server answers with the change. CoreDNS caches answers: the Corefile of the Kubernetes documentation and kubeadm
sets `ttl 30` in the `kubernetes` plugin and `cache 30`, so an answer, NXDOMAIN included, can be up to 30s old. A new pod then reaches clients
within about the refresh interval plus 30s, and pods returning after a full outage within about 30s plus the backoff cap. Lower the plugin's `ttl`
to shorten both; refreshing much more often than the cache expires only adds queries.

A ClusterIP service resolves to one virtual IP that never changes, so lookups never fail and the resolver changes nothing: after an outage the
client reconnects according to its connection backoff, which `transport/grpc/client` exposes as `WithConnectBackoff` (grpc-go's 120s cap by
default). Load spreads over the pods only per connection there, which `MaxConnectionAge` keeps renewing.

## Observability

Metrics (subsystem `grpc_dns_resolver`), labeled by the target as `host:port`. Like the `target` label of the connection pool metrics, its
values are the configured upstreams, so the cardinality stays bounded.

| Name                                | Type    | Labels             | Description                                                              |
|-------------------------------------|---------|--------------------|--------------------------------------------------------------------------|
| `grpc_dns_resolver_lookups_total`   | Counter | `target`, `result` | Lookups by result: `ok`, `empty` (NXDOMAIN or no records) or `error`     |
| `grpc_dns_resolver_addresses`       | Gauge   | `target`           | Addresses of the latest lookup that succeeded or found none              |

Logs: one warning when lookups start failing (`dns resolution failing`, with the error) and one info line when they recover
(`dns resolution recovered`, with the address count, the failed attempts and the downtime). Successful polls are not logged.
