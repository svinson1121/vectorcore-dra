package peermgr

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/svinson1121/vectorcore-dra/internal/config"
	"github.com/svinson1121/vectorcore-dra/internal/diameter/avp"
	"github.com/svinson1121/vectorcore-dra/internal/diameter/message"
	"github.com/svinson1121/vectorcore-dra/internal/diameter/peer"
	"github.com/svinson1121/vectorcore-dra/internal/metrics"
	"github.com/svinson1121/vectorcore-dra/internal/router"
	"github.com/svinson1121/vectorcore-dra/internal/transport"
)

// pendingEntry records an in-flight forwarded request so the answer can be
// returned to the originating peer and latency can be measured.
type pendingEntry struct {
	fromPeer     *peer.Peer
	toPeer       *peer.Peer // target peer; used by sweep to expire on disconnect
	origHopByHop uint32
	forwardedAt  time.Time
	commandCode  uint32
	appID        uint32
}

// RecentMsgRecorder is the interface the manager uses to record routed messages.
// Implemented by api.RecentMsgBuf — defined here as an interface to avoid an import cycle.
type RecentMsgRecorder interface {
	RecordMsg(timestamp time.Time, direction, fromPeer, toPeer string, commandCode uint32, appID uint32, isRequest bool, resultCode uint32, sessionID string)
}

// Manager tracks all running Diameter peers and coordinates their lifecycle.
type Manager struct {
	mu           sync.RWMutex
	peers        map[string]*peer.Peer // keyed by config name
	byAddr       map[string]*peer.Peer // keyed by resolved IP string - for inbound allow-list
	disabledAddr map[string]string     // IP -> peer name, for configured-but-disabled peers
	router       *router.Router
	localID      string // DRA Diameter identity
	log          *zap.Logger
	recorder     RecentMsgRecorder // may be nil

	// pending tracks forwarded requests: DRA-assigned hop-by-hop -> {origin peer, orig HbH}
	pendingMu sync.Mutex
	pending   map[uint32]*pendingEntry
}

// New creates a new Manager.
func New(r *router.Router, localID string, log *zap.Logger) *Manager {
	return &Manager{
		peers:        make(map[string]*peer.Peer),
		byAddr:       make(map[string]*peer.Peer),
		disabledAddr: make(map[string]string),
		router:       r,
		localID:      localID,
		log:          log,
		pending:      make(map[uint32]*pendingEntry),
	}
}

// Start begins background goroutines for the manager (currently: the pending-entry sweep).
// Must be called once after New(), before traffic flows.
func (m *Manager) Start(ctx context.Context) {
	go m.runSweep(ctx)
}

// runSweep runs the pending-entry expiry sweep on a 5-second ticker.
// An entry is expired when its target peer is no longer OPEN (disconnected) or
// when it has been in-flight longer than 30 seconds. Expiring an entry decrements
// the target peer's in-flight counter so the gauge stays accurate.
func (m *Manager) runSweep(ctx context.Context) {
	const sweepInterval = 5 * time.Second
	const answerTimeout = 30 * time.Second

	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.sweepPending(answerTimeout)
		}
	}
}

func (m *Manager) sweepPending(timeout time.Duration) {
	now := time.Now()

	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()

	for hbh, entry := range m.pending {
		peerDown := entry.toPeer.State() != peer.StateOpen
		timedOut := now.Sub(entry.forwardedAt) > timeout

		if peerDown || timedOut {
			entry.toPeer.DecrInFlight()
			delete(m.pending, hbh)

			reason := "timeout"
			if peerDown {
				reason = "peer_down"
			}
			m.log.Debug("expired pending entry",
				zap.Uint32("hop_by_hop", hbh),
				zap.String("from_peer", entry.fromPeer.Cfg().FQDN),
				zap.String("to_peer", entry.toPeer.Cfg().FQDN),
				zap.String("reason", reason),
				zap.Duration("age", now.Sub(entry.forwardedAt)),
			)
		}
	}
}

// SetRecorder wires up a message recorder (called after the API server is created).
func (m *Manager) SetRecorder(r RecentMsgRecorder) {
	m.recorder = r
}

// IsKnownAddress returns true if the remote address belongs to an enabled configured peer.
// Called at Accept() to drop connections from unknown sources before any Diameter
// processing. Matches on IP only (port is ignored - peers may connect from ephemeral ports).
func (m *Manager) IsKnownAddress(addr net.Addr) bool {
	ip := addrToIP(addr)
	if ip == "" {
		return false
	}
	m.mu.RLock()
	_, ok := m.byAddr[ip]
	m.mu.RUnlock()
	return ok
}

// RejectReason returns a human-readable explanation for why a connection from
// addr would be rejected. Used to produce actionable log messages.
func (m *Manager) RejectReason(addr net.Addr) string {
	ip := addrToIP(addr)
	m.mu.RLock()
	name, disabled := m.disabledAddr[ip]
	m.mu.RUnlock()
	if disabled {
		return fmt.Sprintf("peer %q is configured but disabled (enabled: false)", name)
	}
	return "IP not in peer list"
}

// Sync diffs the desired peer configuration against the currently running state
// and applies additions, removals, and restarts.
func (m *Manager) Sync(
	ctx context.Context,
	peers []config.Peer,
	localCfg config.DRAConfig,
	watchdog config.WatchdogConfig,
	reconnect config.ReconnectConfig,
	localIP net.IP,
) {
	// Build desired set (enabled peers only) and track disabled peer IPs for diagnostics.
	desired := make(map[string]config.Peer)
	newDisabled := make(map[string]string) // IP -> name
	for _, p := range peers {
		if p.Enabled {
			desired[p.Name] = p
		} else if p.Address != "" {
			// Store the raw address (may be hostname); store as-is for diagnostic display.
			// We don't resolve DNS here — best-effort diagnostic only.
			newDisabled[p.Address] = p.Name
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.disabledAddr = newDisabled

	// Remove peers not in desired set, or whose config changed
	for name, running := range m.peers {
		cfg, ok := desired[name]
		if !ok || peerConfigChanged(running.Cfg(), cfg) {
			m.log.Info("stopping peer", zap.String("peer", name))
			m.router.RemovePeer(running.Cfg().FQDN)
			running.Stop()
			delete(m.peers, name)
			delete(m.byAddr, running.Cfg().ResolvedIP)
		}
	}

	// Add desired peers that are not yet running
	for name, cfg := range desired {
		if _, ok := m.peers[name]; ok {
			continue // already running with same config
		}

		mode := cfg.Mode
		if mode == "" {
			mode = "active"
		}

		t, err := selectTransport(cfg.Transport, cfg)
		if err != nil {
			m.log.Error("invalid transport for peer, skipping",
				zap.String("peer", name),
				zap.String("transport", cfg.Transport),
				zap.Error(err),
			)
			continue
		}

		initialBackoff := time.Duration(reconnect.InitialBackoffSeconds) * time.Second
		maxBackoff := time.Duration(reconnect.MaxBackoffSeconds) * time.Second

		// Attempt DNS resolution. On failure, do NOT skip the peer - start a
		// background retry loop using the same reconnect backoff timer so it
		// behaves consistently with the Diameter reconnect behaviour (RFC 6733).
		resolvedIP, err := resolveAddress(ctx, cfg.Address)
		if err != nil {
			m.log.Warn("DNS resolution failed, will retry with backoff",
				zap.String("peer", name),
				zap.String("address", cfg.Address),
				zap.Duration("backoff", initialBackoff),
				zap.Error(err),
			)
			go m.dnsRetryLoop(ctx, cfg, localCfg, watchdog, localIP, t, mode, initialBackoff, maxBackoff)
			continue
		}

		m.startPeerLocked(ctx, cfg, localCfg, watchdog, localIP, t, mode, resolvedIP, initialBackoff, maxBackoff)
	}
}

// dnsRetryLoop retries DNS resolution for a peer using the reconnect backoff until
// it succeeds or ctx is cancelled. Once resolved, it calls startPeer to register
// and start the peer normally. This mirrors the Diameter reconnect backoff (RFC 6733)
// so DNS failures are treated the same as connection failures.
func (m *Manager) dnsRetryLoop(
	ctx context.Context,
	cfg config.Peer,
	localCfg config.DRAConfig,
	watchdog config.WatchdogConfig,
	localIP net.IP,
	t transport.Transport,
	mode string,
	initialBackoff, maxBackoff time.Duration,
) {
	backoff := initialBackoff
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		// Check if peer was removed from config while we were waiting
		m.mu.RLock()
		_, stillWanted := m.peers[cfg.Name]
		m.mu.RUnlock()
		// If it now exists (added by a concurrent Sync), stop retrying
		if stillWanted {
			return
		}

		resolvedIP, err := resolveAddress(ctx, cfg.Address)
		if err != nil {
			backoff = nextManagerBackoff(backoff, maxBackoff)
			m.log.Warn("DNS retry failed, backing off",
				zap.String("peer", cfg.Name),
				zap.String("address", cfg.Address),
				zap.Duration("next_backoff", backoff),
				zap.Error(err),
			)
			continue
		}

		m.log.Info("DNS resolved after retry, starting peer",
			zap.String("peer", cfg.Name),
			zap.String("address", cfg.Address),
			zap.String("resolved_ip", resolvedIP),
		)

		m.mu.Lock()
		// Double-check under lock - a concurrent Sync may have already added it
		if _, exists := m.peers[cfg.Name]; !exists {
			m.startPeerLocked(ctx, cfg, localCfg, watchdog, localIP, t, mode, resolvedIP, initialBackoff, maxBackoff)
		}
		m.mu.Unlock()
		return
	}
}

// startPeerLocked registers a resolved peer in the manager maps and starts its FSM.
// MUST be called with m.mu write lock already held.
func (m *Manager) startPeerLocked(
	ctx context.Context,
	cfg config.Peer,
	localCfg config.DRAConfig,
	watchdog config.WatchdogConfig,
	localIP net.IP,
	t transport.Transport,
	mode string,
	resolvedIP string,
	initialBackoff, maxBackoff time.Duration,
) {
	dialAddr := fmt.Sprintf("%s:%d", resolvedIP, cfg.Port)

	pCfg := peer.Config{
		Name:             cfg.Name,
		FQDN:             cfg.FQDN,
		Realm:            cfg.Realm,
		Address:          dialAddr,
		ResolvedIP:       resolvedIP,
		OrigAddress:      cfg.Address,
		Port:             cfg.Port,
		Transport:        t,
		TransportName:    cfg.Transport,
		Mode:             mode,
		PeerGroup:        cfg.PeerGroup,
		Weight:           cfg.Weight,
		LocalFQDN:        localCfg.Identity,
		LocalRealm:       localCfg.Realm,
		LocalIP:          localIP,
		WatchdogInterval: time.Duration(watchdog.IntervalSeconds) * time.Second,
		WatchdogMaxFail:  watchdog.MaxFailures,
		InitialBackoff:   initialBackoff,
		MaxBackoff:       maxBackoff,
		Inbound:          mode == "passive",
	}

	p := peer.New(pCfg, m.log)
	p.OnMessage = m.makeForwarder()
	m.peers[cfg.Name] = p
	m.byAddr[resolvedIP] = p
	m.router.AddPeer(p)

	if mode == "active" {
		p.Start(ctx)
		m.log.Info("peer started (active)",
			zap.String("peer", cfg.Name),
			zap.String("addr", dialAddr),
			zap.String("transport", cfg.Transport),
		)
	} else {
		m.log.Info("peer registered (passive - awaiting inbound connection)",
			zap.String("peer", cfg.Name),
			zap.String("resolved_ip", resolvedIP),
			zap.String("transport", cfg.Transport),
		)
	}
}

// nextManagerBackoff doubles backoff up to max.
func nextManagerBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}

// AcceptInbound is called when an inbound connection is accepted from a known peer.
// transportName is the protocol of the accepting listener (e.g. "tcp", "sctp") — this
// becomes the peer's ActualTransport, which may differ from its configured transport.
func (m *Manager) AcceptInbound(ctx context.Context, conn net.Conn, transportName string, localCfg config.DRAConfig, watchdog config.WatchdogConfig, localIP net.IP) {
	ip := addrToIP(conn.RemoteAddr())

	m.mu.Lock()
	p, ok := m.byAddr[ip]
	m.mu.Unlock()

	if !ok {
		// Shouldn't happen - caller already checked IsKnownAddress
		m.log.Warn("AcceptInbound: no peer for address", zap.String("ip", ip))
		conn.Close()
		return
	}

	state := p.State()
	if state == peer.StateClosed {
		// Accept the inbound connection for both passive and active peers.
		// For active peers this is the RFC 6733 sec 5.6.4 fallback: if the peer
		// beat us to the connection (or our outbound hasn't landed yet), use
		// their inbound connection rather than drop it and force retries.
		p.SetConnWithTransport(conn, transportName)
		p.Start(ctx)
		m.log.Info("inbound connection accepted",
			zap.String("peer", p.Cfg().Name),
			zap.String("mode", p.Cfg().Mode),
			zap.String("remote", conn.RemoteAddr().String()),
		)
	} else {
		// Peer already has an active session - reject the duplicate.
		m.log.Warn("peer already connected, rejecting duplicate inbound",
			zap.String("peer", p.Cfg().Name),
			zap.String("state", state.String()),
			zap.String("remote", conn.RemoteAddr().String()),
		)
		conn.Close()
	}
}

// Forward routes a request msg to the appropriate peer based on routing rules.
// It replaces the hop-by-hop ID with a DRA-generated one, records a pending
// entry so the answer can be matched and returned to src, and inserts a
// Route-Record AVP with the local DRA identity.
func (m *Manager) Forward(src *peer.Peer, msg *message.Message) error {
	target, err := m.router.Route(msg, src.Cfg().FQDN)
	if err != nil {
		return fmt.Errorf("routing failed: %w", err)
	}

	// Replace hop-by-hop with a DRA-generated ID and record the pending entry.
	origHopByHop := msg.Header.HopByHop
	newHopByHop := message.NextHopByHop()
	msg.Header.HopByHop = newHopByHop

	m.pendingMu.Lock()
	m.pending[newHopByHop] = &pendingEntry{
		fromPeer:     src,
		toPeer:       target,
		origHopByHop: origHopByHop,
		forwardedAt:  time.Now(),
		commandCode:  msg.Header.CommandCode,
		appID:        msg.Header.AppID,
	}
	m.pendingMu.Unlock()

	// Insert Route-Record AVP (code 282, DiameterIdentity).
	// RFC 6733 sec 6.7.1: Route-Record MUST NOT have the M-bit set.
	routeRecord := avp.NewString(avp.CodeRouteRecord, 0, 0, m.localID)
	msg.AVPs = append(msg.AVPs, routeRecord)

	target.IncrInFlight()
	cmdStr := strconv.FormatUint(uint64(msg.Header.CommandCode), 10)

	// Record the outbound forwarded request.
	if m.recorder != nil && !isFSMCommand(msg.Header.CommandCode) {
		var sid string
		if sidAVP := msg.FindAVP(avp.CodeSessionID, 0); sidAVP != nil {
			sid, _ = sidAVP.String()
		}
		m.recorder.RecordMsg(time.Now(), "out", src.Cfg().FQDN, target.Cfg().FQDN,
			msg.Header.CommandCode, msg.Header.AppID, true, 0, sid)
	}

	if err := target.Send(msg); err != nil {
		target.DecrInFlight()
		m.pendingMu.Lock()
		delete(m.pending, newHopByHop)
		m.pendingMu.Unlock()
		metrics.MessagesTotal.WithLabelValues(target.Cfg().FQDN, "out", strconv.FormatUint(uint64(msg.Header.AppID), 10), cmdStr, "error").Inc()
		return err
	}
	m.log.Debug("forwarded request",
		zap.String("from", src.Cfg().Name),
		zap.String("to", target.Cfg().Name),
		zap.Uint32("cmd", msg.Header.CommandCode),
		zap.Uint32("app_id", msg.Header.AppID),
		zap.Uint32("hop_by_hop", newHopByHop),
	)
	metrics.MessagesTotal.WithLabelValues(target.Cfg().FQDN, "out", strconv.FormatUint(uint64(msg.Header.AppID), 10), cmdStr, "success").Inc()
	metrics.ForwardedTotal.WithLabelValues(src.Cfg().FQDN, target.Cfg().FQDN, cmdStr).Inc()
	return nil
}

// Get returns the named peer.
func (m *Manager) Get(name string) (*peer.Peer, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.peers[name]
	return p, ok
}

// HasConfiguredPeer returns true if the address belongs to any configured peer
// (passive or active). Used to route inbound connections through AcceptInbound
// rather than creating a transient FSM.
func (m *Manager) HasConfiguredPeer(addr net.Addr) bool {
	ip := addrToIP(addr)
	m.mu.RLock()
	_, ok := m.byAddr[ip]
	m.mu.RUnlock()
	return ok
}

// List returns all current peers.
func (m *Manager) List() []*peer.Peer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*peer.Peer, 0, len(m.peers))
	for _, p := range m.peers {
		out = append(out, p)
	}
	return out
}

// makeForwarder returns an OnMessage handler for a peer.
// For requests: calls Forward to route to the appropriate downstream peer.
// For answers: looks up the pending request table by hop-by-hop ID, restores
// the original hop-by-hop, and relays the answer back to the originating peer.
func (m *Manager) makeForwarder() func(*peer.Peer, *message.Message) {
	return func(src *peer.Peer, msg *message.Message) {
		cmdStr := strconv.FormatUint(uint64(msg.Header.CommandCode), 10)
		appStr := strconv.FormatUint(uint64(msg.Header.AppID), 10)

		if !msg.IsRequest() {
			// src is the downstream peer that answered; decrement its in-flight.
			src.DecrInFlight()

			// Match the answer to the pending forwarded request by hop-by-hop.
			m.pendingMu.Lock()
			entry, ok := m.pending[msg.Header.HopByHop]
			if ok {
				delete(m.pending, msg.Header.HopByHop)
			}
			m.pendingMu.Unlock()

			if !ok {
				m.log.Warn("received answer with no pending request",
					zap.Uint32("cmd", msg.Header.CommandCode),
					zap.Uint32("hop_by_hop", msg.Header.HopByHop),
					zap.String("from_peer", src.Cfg().FQDN),
				)
				metrics.RouteMissesTotal.WithLabelValues("no_pending").Inc()
				return
			}

			// Record answer latency.
			latency := time.Since(entry.forwardedAt).Seconds()
			metrics.AnswerLatency.WithLabelValues(src.Cfg().FQDN, cmdStr, appStr).Observe(latency)
			metrics.MessagesTotal.WithLabelValues(src.Cfg().FQDN, "in", appStr, cmdStr, "success").Inc()

			// Record in recent-messages buffer (skip FSM-only commands).
			if m.recorder != nil && !isFSMCommand(msg.Header.CommandCode) {
				var rc uint32
				if rcAVP := msg.FindAVP(avp.CodeResultCode, 0); rcAVP != nil {
					rc, _ = rcAVP.Uint32()
				}
				var sid string
				if sidAVP := msg.FindAVP(avp.CodeSessionID, 0); sidAVP != nil {
					sid, _ = sidAVP.String()
				}
				m.recorder.RecordMsg(time.Now(), "in", src.Cfg().FQDN, entry.fromPeer.Cfg().FQDN,
					msg.Header.CommandCode, msg.Header.AppID, false, rc, sid)
			}

			// Restore the original hop-by-hop and relay back to the originator.
			msg.Header.HopByHop = entry.origHopByHop
			if err := entry.fromPeer.Send(msg); err != nil {
				m.log.Warn("failed to relay answer to originator",
					zap.Uint32("cmd", msg.Header.CommandCode),
					zap.String("to_peer", entry.fromPeer.Cfg().Name),
					zap.Error(err),
				)
			}
			return
		}

		// Inbound request
		metrics.MessagesTotal.WithLabelValues(src.Cfg().FQDN, "in", appStr, cmdStr, "success").Inc()

		// Record in recent-messages buffer before forwarding (skip FSM-only commands).
		if m.recorder != nil && !isFSMCommand(msg.Header.CommandCode) {
			var sid string
			if sidAVP := msg.FindAVP(avp.CodeSessionID, 0); sidAVP != nil {
				sid, _ = sidAVP.String()
			}
			m.recorder.RecordMsg(time.Now(), "in", src.Cfg().FQDN, "",
				msg.Header.CommandCode, msg.Header.AppID, true, 0, sid)
		}

		if err := m.Forward(src, msg); err != nil {
			m.log.Warn("forward failed",
				zap.String("from_peer", src.Cfg().FQDN),
				zap.Uint32("cmd", msg.Header.CommandCode),
				zap.Error(err),
			)
			// Classify routing miss reason
			reason := "no_rule"
			switch {
			case errors.Is(err, router.ErrNoPeer):
				reason = "no_open_peer"
			case errors.Is(err, router.ErrLoopDetected):
				reason = "loop_detected"
			case errors.Is(err, router.ErrRejected):
				reason = "rejected"
			case errors.Is(err, router.ErrDrop):
				reason = "dropped"
			}
			metrics.RouteMissesTotal.WithLabelValues(reason).Inc()
		}
	}
}

// isFSMCommand returns true for command codes that are FSM-internal and should not
// appear in the recent-messages feed (CER/CEA=257, DWR/DWA=280, DPR/DPA=282).
func isFSMCommand(code uint32) bool {
	return code == 257 || code == 280 || code == 282
}

// peerConfigChanged returns true if any field that would require a restart differs.
func peerConfigChanged(running peer.Config, desired config.Peer) bool {
	return running.OrigAddress != desired.Address ||
		running.Port != desired.Port ||
		running.FQDN != desired.FQDN ||
		running.Realm != desired.Realm ||
		running.TransportName != desired.Transport ||
		running.Mode != desired.Mode
}

// selectTransport returns the appropriate Transport for the given protocol string.
func selectTransport(proto string, cfg config.Peer) (transport.Transport, error) {
	switch proto {
	case "tcp":
		return transport.NewTCP(), nil
	case "tcp+tls":
		// TLS config is not per-peer in current schema - use listener TLS config
		// For now return plain TCP with a note; full TLS requires cert paths on peer
		return transport.NewTCP(), nil
	case "sctp":
		return transport.NewSCTP(), nil
	case "sctp+tls":
		// SCTP+TLS (DTLS per RFC 6083) - deferred; fall back to plain SCTP
		return transport.NewSCTP(), nil
	default:
		return nil, fmt.Errorf("unknown transport %q", proto)
	}
}

// resolveAddress resolves a hostname or IP string to an IP string.
// If addr is already a valid IP, returns it directly.
// Otherwise performs a DNS lookup and returns the first A/AAAA result.
func resolveAddress(ctx context.Context, addr string) (string, error) {
	if ip := net.ParseIP(addr); ip != nil {
		return ip.String(), nil
	}
	// DNS lookup
	ips, err := net.DefaultResolver.LookupHost(ctx, addr)
	if err != nil {
		return "", fmt.Errorf("DNS lookup for %q: %w", addr, err)
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("DNS lookup for %q: no results", addr)
	}
	return ips[0], nil
}

// addrToIP extracts the IP string from a net.Addr, stripping the port.
func addrToIP(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		// addr might be IP-only without port
		return addr.String()
	}
	return host
}
