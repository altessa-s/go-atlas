# ADR 0002. The gRPC client resolves dns targets with its own resolver

Accepted on 2026-11-24.

## Context

`transport/grpc/client` passes the address it is given to `grpc.NewClient`. A target without a scheme such as `inventory.default.svc:9000`
gets the default scheme `dns` (grpc-go v1.85.0 `dialoptions.go:730`, `clientconn.go:1846-1851`) and is resolved by grpc-go's built-in
dns resolver. The client's default service config selects `round_robin`, which grpc-go builds on `balancer/endpointsharding`
(`balancer/roundrobin/roundrobin.go:53`). The connection backoff was fixed to `backoff.DefaultConfig`: 1s, ×1.6, jitter 0.2, 120s cap
(`backoff/backoff.go:47-51`).

A headless Kubernetes service resolves to one A record per ready pod. Two failures follow from the built-in resolver
(`internal/resolver/dns/dns_resolver.go`):

1. Slow recovery after a full outage. With every pod gone, DNS answers NXDOMAIN or no records. `handleDNSError` (`:278-291`) drops errors that
   are neither timeouts nor temporary, so `lookupHost` (`:321-337`) returns an empty list without error and `lookup` (`:339-364`) reports a
   state without addresses. `endpointsharding` rejects it with `balancer.ErrBadResolverState` (`endpointsharding.go:186-188`) and its picker
   fails RPCs with `no children to pick from` (`:292`). The watcher (`:206-241`) then takes its error branch: the next lookup waits
   `backoff.DefaultExponential` (`:232`; `internal/backoff/backoff.go:45`, up to 120s), and the wait (`:235-239`) selects only on the context
   and the timer, not on `ResolveNow`. After an outage of about 70s the next lookup can be 40-120s away. On a Kubernetes cluster a 70s
   outage produced more than 60s of failed RPCs after the servers were ready again; a short outage or a rolling restart recovered in 2-4s.
2. Invisible scale-up. After a successful lookup the watcher waits for `ResolveNow` (`:219-228`), at most once per `MinResolutionInterval`
   (30s, `:52`), and never polls. `ResolveNow` comes only from a failing subchannel, so after a scale from 2 to 4 pods existing clients keep
   using 2 until a connection breaks, for example at the server's `MaxConnectionAge` (30m by default in `config/grpc`).

grpc-go also looks up service configs in TXT records by default (`envconfig.go:32`; `dns_resolver.go:144`, `:293-319`, `:360-362`). A
service config from the resolver replaces the default one (`clientconn.go:833-847`), so a `_grpc_config.<host>` TXT record would remove
the client's round-robin balancing, health checking and retries.

## Decision

`transport/grpc/client/dnsresolver` implements the `dns` scheme. The client passes a builder to each of its connections with
`grpc.WithResolvers`, which a connection consults before the global registry (`dialoptions.go:765-769`, `clientconn.go:1799-1806`); pooled
connections receive it through the same dial options. The package never calls `resolver.Register`. Targets with other schemes are untouched,
a `dns` builder passed through `WithDialOptions` takes precedence, and `WithBuiltinDNSResolver` restores grpc-go's resolver.

The resolver accepts the target forms of grpc-go's and looks up A and AAAA records with a timeout (10s). While lookups succeed it
re-resolves every 30s, shortened at random by up to 10% so that clients started together drift apart, and reports only a changed address set;
`ResolveNow` brings a lookup forward, no sooner than 2s after the previous one.

A failed lookup (a timeout, SERVFAIL or another error, also of only one of its queries, through `net.Resolver.StrictErrors`) is reported with
`ReportError`, and the connection keeps its addresses. An answer that no address exists (NXDOMAIN or no records), which is what DNS gives for a
headless service without ready pods, first drops the addresses with an empty state and is then reported the same way. The dropped addresses'
subchannels close, so servers that return are connected at once, also on their old addresses: a container restarted in place keeps its pod IP,
and its old subchannel would wait out a connection backoff of up to 120s. Envoy's `STRICT_DNS` clusters and Finagle treat the two answers the
same way. Both are retried with jittered exponential backoff from 1s capped at 5s, where the jitter only shortens the delay; `ResolveNow`
shortens the wait. A state the connection rejects is retried the same way. Recovery is reported at once. The resolver does not look up SRV or
TXT records, so the client's default service config always applies.

The connection backoff becomes configurable (`WithConnectBackoff`) with grpc-go's values as defaults. The `grpcclientconfig` schema and
`factory.ConnectionOptions` expose both.

## Alternatives considered

- `resolver/dns.SetMinResolutionInterval` and `SetResolvingTimeout` (`resolver/dns/dns_resolver.go:42-59`) set process-wide variables for
  every gRPC user of the process. They leave the error backoff and the missing polling unchanged, so they fix neither failure.
- Relying on the server's `MaxConnectionAge`. It eventually spreads clients over new pods, but only after the age (30m by default), and it
  does nothing for the empty-answer backoff.
- ClusterIP services. The virtual IP hides pod changes from the resolver, but gRPC keeps long-lived connections, so load spreads over pods
  only per connection, and recovery after an outage then depends on the connection backoff, whose default cap is 120s.
- Keeping the addresses on an answer that none exists, as on a failed lookup. It would survive a spurious NXDOMAIN, but the old subchannels
  then stay in connection backoff, so after a full outage pods returning on their old IPs are reached only when that backoff expires, and RPCs
  keep going to pods that are alive but no longer ready.
- A resolver watching Kubernetes Endpoints. It reacts fastest, but it ties a framework-neutral toolkit to the Kubernetes API, credentials and
  RBAC, and DNS works for every deployment.

## Consequences

Clients see new pods of a headless service within the refresh interval and recover within the backoff cap after a full outage, whether the
pods return on new addresses or on their old ones. Both bounds count from the DNS answer: CoreDNS with the Corefile of the Kubernetes
documentation (`ttl 30`, `cache 30`) serves answers, NXDOMAIN included, up to 30s old. A spurious NXDOMAIN closes a target's connections until
DNS answers again. Each
connection performs one lookup per refresh interval while healthy and one per backoff step while failing, so a pool of N connections to a
target multiplies that load by N. Deployments that published service configs in `_grpc_config` TXT records lose them unless they use
`WithBuiltinDNSResolver`. With ClusterIP services or IP targets the resolver never fails; recovery there is governed by the connection
backoff, which operators can now lower. Other gRPC users of the process keep grpc-go's resolver.
