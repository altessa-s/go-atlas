# ADR 0002. The gRPC client offers an opt-in DNS resolver

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

The client follows grpc-go's resolver selection by default to preserve DNS TXT service configs and globally registered builders. Enable the
Atlas DNS resolver with `WithDNSResolver()` or `grpcClient.resolver.enabled: true`. When enabled, the client installs the Atlas builder with
`grpc.WithResolvers` on each connection, including pooled connections. An application-supplied `dns` builder in `WithDialOptions` takes
precedence. The global registry still serves other clients, and other schemes retain their normal resolver selection.

The Atlas resolver queries A and AAAA independently within a shared 10s timeout. A transient failure keeps that family's previous addresses
while any usable answers from the other family can be published. While lookups succeed, the resolver refreshes every 30s by default, shortened
at random by up to 10%. `ResolveNow` brings a lookup forward, no sooner than 2s after the previous one. A scheduled lookup satisfies a pending
request.

An empty state is submitted only when both families conclusively return no addresses. The default round-robin policy then removes its
subchannels. A returning server gets a fresh subchannel even on its old address. Other policies may reject the empty state and keep their
subchannels. Failed and empty lookups are reported with `ReportError` and retried with jittered exponential backoff from 1s, capped at 5s.
`ResolveNow` can shorten the wait. Recovery is reported at once. The Atlas resolver performs address lookups only, so the client's default
service config applies. Resolver metrics aggregate across all targets on the meter.

`WithConnectBackoff` configures connection backoff independently of DNS resolution. grpc-go applies jitter after this cap, so later
connection waits can exceed it. The `grpcclientconfig` schema and `factory.ConnectionOptions` expose both settings.

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

Clients that enable the Atlas resolver see new pods of a headless service within the refresh interval. After a full outage, the resolver
can see returning pods within the backoff cap. These bounds start when DNS serves the new answer. CoreDNS with the Kubernetes example
configuration (`ttl 30`, `cache 30`) can serve cached answers, including NXDOMAIN, for up to 30s. With round-robin, a spurious negative
in both families removes the old subchannels until DNS answers again.

Each connection queries both address families every refresh interval while healthy and every backoff step while failing. A pool of N
connections multiplies that load by N. Deployments using DNS TXT service configs keep them with the default resolver selection. For a
ClusterIP service or IP target, connection backoff controls reconnection while the resolved address stays the same.
