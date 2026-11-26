// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nkeys"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/observability/health"

	coremaps "github.com/altessa-s/go-atlas-core/collections/maps"
	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	natsconfig "github.com/altessa-s/go-atlas/config/nats"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

const (
	// UnlimitedReconnects indicates that reconnection attempts should continue
	// indefinitely. Passed to [nats.MaxReconnects] when no explicit limit is
	// configured.
	UnlimitedReconnects = -1
)

// Policy conversion maps translate [config] policy constants to their
// [jetstream] equivalents for consumer configuration.
var (
	deliverPolicyMap = coremaps.NewImmutableMap(map[natsconfig.DeliverPolicy]jetstream.DeliverPolicy{
		natsconfig.DeliverPolicyAll:             jetstream.DeliverAllPolicy,
		natsconfig.DeliverPolicyLast:            jetstream.DeliverLastPolicy,
		natsconfig.DeliverPolicyNew:             jetstream.DeliverNewPolicy,
		natsconfig.DeliverPolicyByStartSequence: jetstream.DeliverByStartSequencePolicy,
		natsconfig.DeliverPolicyByStartTime:     jetstream.DeliverByStartTimePolicy,
	})

	ackPolicyMap = coremaps.NewImmutableMap(map[natsconfig.AckPolicy]jetstream.AckPolicy{
		natsconfig.AckPolicyNone:     jetstream.AckNonePolicy,
		natsconfig.AckPolicyAll:      jetstream.AckAllPolicy,
		natsconfig.AckPolicyExplicit: jetstream.AckExplicitPolicy,
	})

	replayPolicyMap = coremaps.NewImmutableMap(map[natsconfig.ReplayPolicy]jetstream.ReplayPolicy{
		natsconfig.ReplayPolicyInstant:  jetstream.ReplayInstantPolicy,
		natsconfig.ReplayPolicyOriginal: jetstream.ReplayOriginalPolicy,
	})
)

// natsReconnectHandler logs NATS reconnection. Used by NATSOptions to avoid
// allocating handler logic in hot path; the returned option still captures logger.
func natsReconnectHandler(logger *slog.Logger) func(*nats.Conn) {
	return func(conn *nats.Conn) {
		url := conn.ConnectedUrlRedacted()
		logger.Debug("NATS connection re-established", slog.String("url", url))
	}
}

// natsDisconnectErrHandler logs NATS disconnect events. Used by NATSOptions.
func natsDisconnectErrHandler(logger *slog.Logger) func(*nats.Conn, error) {
	return func(_ *nats.Conn, err error) {
		if err != nil {
			logger.Warn("NATS connection lost", slog.Any("error", err))
			return
		}
		logger.Info("NATS connection disconnected")
	}
}

// natsErrorHandler logs NATS connection and subscription errors. Used by NATSOptions.
func natsErrorHandler(logger *slog.Logger) func(*nats.Conn, *nats.Subscription, error) {
	return func(_ *nats.Conn, subscription *nats.Subscription, err error) {
		if subscription != nil {
			logger.Error("NATS subscription error",
				slog.Any("error", err),
				slog.String("subject", subscription.Subject))
		} else {
			logger.Error("NATS connection error", slog.Any("error", err))
		}
	}
}

// ConnectionBuilder assembles a NATS [nats.Conn] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [ConnectionBuilder.Build] time.
// The builder is not safe for concurrent use.
type ConnectionBuilder struct {
	corefactory.Base
	cfg  *natsconfig.Config
	errs []error

	// Dependencies
	tlsConfig         *tls.Config
	healthCoordinator health.Registrar
	healthServiceName string
}

// New creates a [ConnectionBuilder] for the given NATS config.
// Config can be nil — the error surfaces at [ConnectionBuilder.Build] time.
func New(cfg *natsconfig.Config) *ConnectionBuilder {
	return &ConnectionBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build creates a NATS connection from configuration.
// When [natsconfig.Config.ConnectionURI] is set, it is used directly as the
// connection URL. Otherwise, the URL is built by joining the configured Hosts.
// If a [health.Registrar] was provided via [ConnectionBuilder.UseHealthCoordinator],
// a health checker is registered under the service name "nats". The managed
// connection releases it on Close or when the underlying connection closes.
func (b *ConnectionBuilder) Build() (*ManagedConn, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	url := strings.Join(b.cfg.Hosts, ",")
	if b.cfg.UseConnectionURI() {
		url = b.cfg.ConnectionURI.Expose()
	}

	opts, err := b.NATSOptions()
	if err != nil {
		return nil, err
	}
	conn, err := nats.Connect(url, opts...)
	if err != nil {
		// nats.Connect's error can embed the connection URL verbatim — which
		// may carry inline userinfo (nats://user:pass@host), and it references
		// individual servers, not the comma-joined list. Scrub every raw host
		// occurrence from the error text before returning so credentials cannot
		// leak through whatever log path picks the error up. The wrapped chain is
		// dropped intentionally (callers do not match nats.Connect's internal
		// errors), which is the price of guaranteeing no credential leak.
		return nil, fmt.Errorf("connect to NATS at %s: %s", redactNATSURL(url), scrubURLCredentials(err.Error(), url))
	}

	managed := &ManagedConn{Conn: conn}
	if !nilcheck.IsNil(b.healthCoordinator) {
		managed.unregister = sync.OnceFunc(health.RegisterOwnedChecker(b.healthCoordinator, cmp.Or(b.healthServiceName, "nats"), &natsHealthChecker{
			conn:   conn,
			logger: b.Logger(),
		}))
	}
	conn.SetClosedHandler(func(closed *nats.Conn) {
		managed.releaseHealth()
		if handler := managed.ClosedHandler(); handler != nil {
			handler(closed)
		}
	})
	if conn.IsClosed() {
		managed.releaseHealth()
	}
	return managed, nil
}

// ManagedConn owns a NATS connection and its health registration.
// Use its lifecycle and callback methods when passing the embedded Conn to NATS APIs.
type ManagedConn struct {
	*nats.Conn
	unregister    func()
	callbackMu    sync.RWMutex
	closedHandler nats.ConnHandler
}

// Close closes the connection and synchronously releases its health registration.
func (c *ManagedConn) Close() {
	c.Conn.Close()
	c.releaseHealth()
}

func (c *ManagedConn) releaseHealth() {
	if c.unregister != nil {
		c.unregister()
	}
}

// SetClosedHandler sets the caller callback invoked when the connection closes.
func (c *ManagedConn) SetClosedHandler(handler nats.ConnHandler) {
	c.callbackMu.Lock()
	defer c.callbackMu.Unlock()
	c.closedHandler = handler
}

// ClosedHandler returns the caller callback for connection closure.
func (c *ManagedConn) ClosedHandler() nats.ConnHandler {
	c.callbackMu.RLock()
	defer c.callbackMu.RUnlock()
	return c.closedHandler
}

// NATSOptions creates [nats.Option] values from configuration.
// It configures timeouts, reconnection behavior (unlimited by default),
// compression, and logging handlers for disconnect/reconnect/error events.
//
// Authentication is selected automatically based on which credential fields
// are set in [natsconfig.Config]: NKey seed, token, or username/password.
// When [natsconfig.Config.ConnectionURI] is set, authentication fields are ignored
// because they are embedded in the URI.
func (b *ConnectionBuilder) NATSOptions() ([]nats.Option, error) {
	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	logger := b.Logger()

	maxReconnect := cmp.Or(b.cfg.MaxReconnect, UnlimitedReconnects)

	natsOptions := []nats.Option{
		nats.ReconnectBufSize(nats.DefaultReconnectBufSize),
		nats.Compression(true),
		nats.RetryOnFailedConnect(true),
		nats.Name(b.cfg.ClientName),
		nats.Timeout(b.cfg.ConnectTimeout),
		nats.MaxReconnects(maxReconnect),
		nats.ReconnectWait(b.cfg.ReconnectWait),
		nats.PingInterval(b.cfg.PingInterval),
		nats.MaxPingsOutstanding(b.cfg.MaxPingsOut),
		nats.ReconnectHandler(natsReconnectHandler(logger)),
		nats.DisconnectErrHandler(natsDisconnectErrHandler(logger)),
		nats.ErrorHandler(natsErrorHandler(logger)),
	}

	natsOptions = coreslices.AppendIf(natsOptions, b.tlsConfig != nil, nats.Secure(b.tlsConfig))

	if !b.cfg.UseConnectionURI() {
		switch {
		case b.cfg.NkeySeed != "":
			kp, err := nkeys.FromSeed([]byte(b.cfg.NkeySeed.Expose()))
			if err != nil {
				return nil, b.WrapError(err, "failed to parse nkey seed")
			}
			pub, err := kp.PublicKey()
			if err != nil {
				return nil, b.WrapError(err, "failed to derive nkey public key")
			}
			natsOptions = append(natsOptions, nats.Nkey(pub, kp.Sign))
		case b.cfg.Token != "":
			natsOptions = append(natsOptions, nats.Token(b.cfg.Token.Expose()))
		case b.cfg.Username != "" && b.cfg.Password != "":
			natsOptions = append(natsOptions, nats.UserInfo(b.cfg.Username, b.cfg.Password.Expose()))
		}
	}

	return natsOptions, nil
}

// ConsumerConfig converts [natsconfig.Consumer] to [jetstream.ConsumerConfig].
// It maps delivery policy, ack policy, replay policy, filter subjects, backoff
// schedules, and performance tuning. When the delivery policy is
// [natsconfig.DeliverPolicyByStartSequence] or [natsconfig.DeliverPolicyByStartTime],
// the corresponding OptStartSeq or OptStartTime field is set.
func ConsumerConfig(
	cfg *natsconfig.Consumer,
) (*jetstream.ConsumerConfig, error) {
	if cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}
	// A missing key yields the zero policy, matching the former plain-map lookup.
	deliverPolicy, _ := deliverPolicyMap.Get(cfg.DeliverPolicy)
	ackPolicy, _ := ackPolicyMap.Get(cfg.AckPolicy)
	replayPolicy, _ := replayPolicyMap.Get(cfg.ReplayPolicy)
	consumerCfg := &jetstream.ConsumerConfig{
		Description:       cfg.Description,
		FilterSubjects:    cfg.FilterSubjects,
		MaxAckPending:     cfg.MaxAckPending,
		MaxWaiting:        cfg.MaxWaiting,
		InactiveThreshold: cfg.InactiveThreshold,
		AckWait:           cfg.AckWait,
		MaxDeliver:        cfg.MaxDeliver,
		BackOff:           cfg.BackOff,
		DeliverPolicy:     deliverPolicy,
		AckPolicy:         ackPolicy,
		ReplayPolicy:      replayPolicy,
		Durable:           cfg.DurableName,
		Name:              cfg.DurableName,
	}

	if cfg.DeliverPolicy == natsconfig.DeliverPolicyByStartSequence {
		consumerCfg.OptStartSeq = cfg.OptStartSeq
	}

	if cfg.DeliverPolicy == natsconfig.DeliverPolicyByStartTime {
		consumerCfg.OptStartTime = cfg.OptStartTime
	}

	return consumerCfg, nil
}

// redactNATSURL strips userinfo (user[:password]@) from a NATS
// connection string before it is logged or returned in an error.
// Multi-URL comma-separated lists are handled — each server entry is
// redacted independently. The redaction is textual and never parses the
// URL, so a component that is not a valid URL (an unclosed IPv6 bracket,
// an unencoded "/", "@" or "," in the password) loses its userinfo too.
func redactNATSURL(raw string) string {
	entries := natsURLEntries(raw)
	for i, e := range entries {
		entries[i] = stripUserinfo(e)
	}
	return strings.Join(entries, ",")
}

// natsURLEntries splits a comma-separated NATS server list into server
// entries. A password may hold an unencoded ",", so a comma-separated part
// without "@" is joined with the following parts up to the first one holding
// "@", unless a part carrying a scheme starts the next server first. A
// schemeless host list followed by a userinfo entry ("h1:4222,u:p@h2") is
// therefore taken as one entry and over-redacted ("h2"), which loses a host
// from the message rather than risk leaving part of a password behind.
func natsURLEntries(raw string) []string {
	parts := strings.Split(raw, ",")
	entries := make([]string, 0, len(parts))
	for i := 0; i < len(parts); {
		end := userinfoEnd(parts, i)
		entries = append(entries, strings.Join(parts[i:end+1], ","))
		i = end + 1
	}
	return entries
}

// userinfoEnd returns the index of the part that ends the server entry
// starting at parts[i]: the first part from i holding "@", or i itself when
// none does before the next part carrying a scheme.
func userinfoEnd(parts []string, i int) int {
	for k := i; k < len(parts); k++ {
		if k > i && strings.Contains(parts[k], "://") {
			break
		}
		if strings.Contains(parts[k], "@") {
			return k
		}
	}
	return i
}

// stripUserinfo removes everything between the scheme separator (or the
// start of the component, past leading spaces) and the last "@". Cutting
// at the last "@" over-redacts a path or query holding "@", which NATS
// URLs do not use, rather than risk leaving part of a password behind.
func stripUserinfo(s string) string {
	at := strings.LastIndexByte(s, '@')
	if at < 0 {
		return s
	}
	start := len(s) - len(strings.TrimLeft(s, " \t"))
	if i := strings.Index(s, "://"); i >= 0 && i < at {
		start = i + len("://")
	}
	return s[:start] + s[at+1:]
}

// withheldConnectError replaces a connect error whose text cannot be
// scrubbed reliably.
const withheldConnectError = "error text withheld: a server URL carries userinfo nats.go cannot parse, " +
	"and its parse error may echo part of the password"

// scrubURLCredentials removes inline userinfo from every occurrence of a raw host
// (taken from the comma-separated url) in msg, replacing it with the redacted
// form. It is used to sanitize error strings that may echo a connection URL
// carrying credentials.
//
// nats.go splits the list at every comma and parses each part, and its parse
// error echoes fragments of the part (`invalid port ":<password prefix>"`).
// Such fragments cannot be found by replacing whole URLs, so when a server
// entry with userinfo spans a comma or does not parse, msg is withheld.
func scrubURLCredentials(msg, url string) string {
	for _, raw := range natsURLEntries(url) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if strings.Contains(raw, "@") && (strings.Contains(raw, ",") || !parsesAsNATSURL(raw)) {
			return withheldConnectError
		}
		msg = strings.ReplaceAll(msg, raw, redactNATSURL(raw))
	}
	return msg
}

// parsesAsNATSURL reports whether nats.go would parse raw as a server URL; like
// nats.go, it assumes the nats scheme when raw has none.
func parsesAsNATSURL(raw string) bool {
	if !strings.Contains(raw, "://") {
		raw = "nats://" + raw
	}
	_, err := url.Parse(raw)
	return err == nil
}
