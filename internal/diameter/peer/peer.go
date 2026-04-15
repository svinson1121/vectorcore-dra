package peer

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/svinson1121/vectorcore-dra/internal/diameter/message"
	"github.com/svinson1121/vectorcore-dra/internal/metrics"
	"github.com/svinson1121/vectorcore-dra/internal/transport"
)

// State represents the FSM state of a peer connection.
type State int

const (
	StateClosed      State = iota
	StateWaitConnAck       // TCP connect in progress
	StateWaitCEA           // CER sent, awaiting CEA
	StateOpen              // CER/CEA complete, peer is operational
	StateClosing           // DPR sent or received, draining
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateWaitConnAck:
		return "WAIT_CONN_ACK"
	case StateWaitCEA:
		return "WAIT_CEA"
	case StateOpen:
		return "OPEN"
	case StateClosing:
		return "CLOSING"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", int(s))
	}
}

// Config holds per-peer configuration.
type Config struct {
	Name          string // human label / unique key
	FQDN          string
	Realm         string
	Address       string // "resolvedIP:port" - used for dialing
	ResolvedIP    string // resolved IP string - used for inbound allow-list keying
	OrigAddress   string // original address field from config (IP or FQDN)
	Port          int
	Transport     transport.Transport
	TransportName string // "tcp" | "tcp+tls" | "sctp" | "sctp+tls"
	Mode    string // "active" | "passive"
	LBGroup string // lb group name this peer belongs to
	Weight        int    // for weighted load balancing

	// Local DRA identity
	LocalFQDN  string
	LocalRealm string
	LocalIP    net.IP

	// Watchdog settings
	WatchdogInterval time.Duration
	WatchdogMaxFail  int

	// Reconnect backoff
	InitialBackoff time.Duration
	MaxBackoff     time.Duration

	// Inbound indicates this peer was accepted (not dialed); true for passive mode
	Inbound bool
}

// Peer represents a remote Diameter peer with a full FSM lifecycle.
type Peer struct {
	cfg Config

	mu              sync.RWMutex
	state           State
	conn            net.Conn
	ActualTransport string    // transport actually in use; set at connect time
	connectedAt     time.Time // when state last became OPEN

	writeCh chan *message.Message
	stopCh  chan struct{}
	stopped bool

	// Capabilities learned during CER/CEA
	PeerFQDN   string
	PeerRealm  string
	PeerAppIDs []uint32

	// Watchdog tracking - all state owned by watchdogLoop goroutine, signalled via wdReplyCh
	wdReplyCh chan struct{}

	// CEA signal: writeloop posts here when CEA is received
	ceaCh chan error

	// DPR signal: signals the FSM to close after DPA
	dprDoneCh chan struct{}

	// In-flight request counter (for least_conn load balancing)
	inFlight int64

	// Message handler - called for non-FSM messages
	OnMessage func(p *Peer, msg *message.Message)

	log *zap.Logger
}

// New creates a new Peer.
func New(cfg Config, log *zap.Logger) *Peer {
	return &Peer{
		cfg:             cfg,
		state:           StateClosed,
		ActualTransport: cfg.TransportName, // correct for outbound; overridden via SetConnWithTransport for inbound
		writeCh:         make(chan *message.Message, 64),
		stopCh:          make(chan struct{}),
		ceaCh:           make(chan error, 1),
		dprDoneCh:       make(chan struct{}, 1),
		wdReplyCh:       make(chan struct{}, 1),
		log:             log.With(zap.String("peer", cfg.FQDN), zap.String("addr", cfg.Address)),
	}
}

// Start begins the peer connect loop in a background goroutine.
// For inbound peers (Inbound=true), conn must be set before calling Start.
func (p *Peer) Start(ctx context.Context) {
	if p.cfg.Inbound {
		go p.runInbound(ctx)
	} else {
		go p.runOutbound(ctx)
	}
}

// Stop initiates graceful shutdown: sends DPR, waits for DPA, then closes.
func (p *Peer) Stop() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	p.mu.Unlock()
	close(p.stopCh)
}

// Send queues a message for delivery to the peer.
func (p *Peer) Send(msg *message.Message) error {
	p.mu.RLock()
	state := p.state
	p.mu.RUnlock()

	if state != StateOpen {
		return fmt.Errorf("peer %s: not in OPEN state (current: %s)", p.cfg.FQDN, state)
	}

	select {
	case p.writeCh <- msg:
		return nil
	default:
		return fmt.Errorf("peer %s: write channel full", p.cfg.FQDN)
	}
}

// State returns the current FSM state.
func (p *Peer) State() State {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

// setState sets the FSM state under the write lock and updates the Prometheus gauge.
func (p *Peer) setState(s State) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state = s
	p.log.Debug("peer state change", zap.String("state", s.String()))

	// Update Prometheus peer state gauge:
	// 0=closed, 1=connecting, 2=open, 3=draining
	stateVal := float64(0)
	switch s {
	case StateWaitConnAck, StateWaitCEA:
		stateVal = 1
	case StateOpen:
		stateVal = 2
		p.connectedAt = time.Now()
		metrics.PeerConnectsTotal.WithLabelValues(p.cfg.FQDN, p.cfg.Mode).Inc()
	case StateClosing:
		stateVal = 3
	}
	metrics.PeerState.WithLabelValues(p.cfg.FQDN, p.cfg.TransportName, p.cfg.Mode).Set(stateVal)
}

// String returns a human-readable description of the peer.
func (p *Peer) String() string {
	return fmt.Sprintf("Peer{fqdn=%s addr=%s state=%s}", p.cfg.FQDN, p.cfg.Address, p.State())
}

// Cfg returns a copy of the peer's configuration.
func (p *Peer) Cfg() Config {
	return p.cfg
}

// InFlight returns the current number of in-flight requests for this peer.
func (p *Peer) InFlight() int64 {
	return atomic.LoadInt64(&p.inFlight)
}

// IncrInFlight increments the in-flight counter.
func (p *Peer) IncrInFlight() {
	atomic.AddInt64(&p.inFlight, 1)
}

// DecrInFlight decrements the in-flight counter.
func (p *Peer) DecrInFlight() {
	atomic.AddInt64(&p.inFlight, -1)
}

// SetConn sets the underlying connection (for inbound peers accepted by a listener).
func (p *Peer) SetConn(conn net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.conn = conn
}

// SetConnWithTransport sets the connection and records the actual transport protocol.
// Use this for inbound connections where the transport may differ from the configured value.
func (p *Peer) SetConnWithTransport(conn net.Conn, transportName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.conn = conn
	p.ActualTransport = transportName
}

// ConnectedAt returns when the peer last reached StateOpen (zero if never).
func (p *Peer) ConnectedAt() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.connectedAt
}

// RemoteAddr returns the remote address of the current connection, or "" if not connected.
func (p *Peer) RemoteAddr() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.conn == nil {
		return ""
	}
	return p.conn.RemoteAddr().String()
}

// runOutbound manages the full outbound lifecycle with reconnect backoff.
// DNS resolution (if the peer address is a hostname) happens on each attempt.
func (p *Peer) runOutbound(ctx context.Context) {
	backoff := p.cfg.InitialBackoff
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		default:
		}

		p.setState(StateWaitConnAck)

		// Re-resolve DNS on each attempt if OrigAddress is a hostname
		dialAddr := p.cfg.Address
		if p.cfg.OrigAddress != "" {
			resolved, err := resolveForDial(ctx, p.cfg.OrigAddress, p.cfg.Port)
			if err != nil {
				p.setState(StateClosed)
				p.log.Warn("DNS resolution failed", zap.Error(err), zap.Duration("backoff", backoff))
				select {
				case <-ctx.Done():
					return
				case <-p.stopCh:
					return
				case <-time.After(backoff):
				}
				backoff = nextBackoff(backoff, p.cfg.MaxBackoff)
				continue
			}
			dialAddr = resolved
		}

		p.log.Info("connecting to peer", zap.String("dial_addr", dialAddr))

		conn, err := p.cfg.Transport.Dial(ctx, dialAddr)
		if err != nil {
			p.setState(StateClosed)
			p.log.Warn("connection failed", zap.Error(err), zap.Duration("backoff", backoff))
			select {
			case <-ctx.Done():
				return
			case <-p.stopCh:
				return
			case <-time.After(backoff):
			}
			backoff = nextBackoff(backoff, p.cfg.MaxBackoff)
			continue
		}

		p.mu.Lock()
		p.conn = conn
		p.mu.Unlock()

		p.log.Info("connected", zap.String("remote", conn.RemoteAddr().String()))
		err = p.runSession(ctx)

		// Clear the connection reference immediately so RemoteAddr() returns ""
		// rather than calling into a closed/invalid socket (SCTP panics on closed fd).
		p.mu.Lock()
		p.conn = nil
		p.mu.Unlock()

		if err != nil {
			p.log.Warn("session ended with error", zap.Error(err))
		} else {
			p.log.Info("session ended cleanly")
		}

		// Check if stop was requested
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		default:
		}

		p.setState(StateClosed)
		metrics.ReconnectAttemptsTotal.WithLabelValues(p.cfg.FQDN).Inc()
		metrics.ReconnectBackoffSeconds.WithLabelValues(p.cfg.FQDN).Set(backoff.Seconds())
		p.log.Info("reconnecting after backoff", zap.Duration("backoff", backoff))
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		case <-time.After(backoff):
		}
		backoff = nextBackoff(backoff, p.cfg.MaxBackoff)
	}
}

// runInbound manages the lifecycle of an inbound peer connection.
func (p *Peer) runInbound(ctx context.Context) {
	p.log.Info("inbound peer session starting")
	err := p.runSession(ctx)

	// Clear the connection reference before setState so RemoteAddr() is safe to call.
	p.mu.Lock()
	p.conn = nil
	p.mu.Unlock()

	if err != nil {
		p.log.Warn("inbound session ended with error", zap.Error(err))
	} else {
		p.log.Info("inbound session ended cleanly")
	}
	p.setState(StateClosed)
}

// runSession runs the Diameter session for an already-connected peer.
// This includes CER/CEA, the watchdog loop, and the read/write loops.
func (p *Peer) runSession(ctx context.Context) error {
	conn := p.conn

	// sessionDone is closed when this session ends. The write loop listens on
	// it so it exits before the next session starts its own write loop.
	// Without this, an old write loop goroutine can outlive the session and
	// race with the new session's write loop to consume messages from writeCh,
	// silently dropping whichever message the old goroutine dequeues
	// (writes to the closed connection fail and the message is lost).
	sessionDone := make(chan struct{})
	defer close(sessionDone)

	// Drain stale signals from previous session
	select {
	case <-p.wdReplyCh:
	default:
	}
	select {
	case <-p.ceaCh:
	default:
	}

	// Start the read and write loops
	readErrCh := make(chan error, 1)
	writeErrCh := make(chan error, 1)
	go p.writeLoop(ctx, conn, writeErrCh, sessionDone)
	go p.readLoop(ctx, conn, readErrCh)

	// Initiate CER or wait for CER depending on direction
	if !p.cfg.Inbound {
		p.setState(StateWaitCEA)
		if err := sendCER(p); err != nil {
			conn.Close()
			return fmt.Errorf("sending CER: %w", err)
		}

		// Wait for CEA
		select {
		case err := <-p.ceaCh:
			if err != nil {
				conn.Close()
				return fmt.Errorf("CER/CEA exchange: %w", err)
			}
		case <-time.After(10 * time.Second):
			conn.Close()
			return fmt.Errorf("timeout waiting for CEA")
		case <-ctx.Done():
			conn.Close()
			return ctx.Err()
		case <-p.stopCh:
			conn.Close()
			return nil
		}
	} else {
		// For inbound: wait for CER to arrive (handled in readLoop/dispatch)
		// The readLoop will call handleCER which sends CEA and signals ceaCh
		p.setState(StateWaitCEA)
		select {
		case err := <-p.ceaCh:
			if err != nil {
				conn.Close()
				return fmt.Errorf("inbound CER/CEA: %w", err)
			}
		case <-time.After(10 * time.Second):
			conn.Close()
			return fmt.Errorf("timeout waiting for inbound CER")
		case <-ctx.Done():
			conn.Close()
			return ctx.Err()
		case <-p.stopCh:
			conn.Close()
			return nil
		}
	}

	p.setState(StateOpen)
	p.log.Info("peer is OPEN", zap.String("peerFQDN", p.PeerFQDN), zap.String("peerRealm", p.PeerRealm))

	// Reset backoff tracking (done in caller by successful session)
	// Start watchdog
	wdDone := make(chan struct{})
	go p.watchdogLoop(ctx, wdDone)

	// Wait for stop, context cancel, or I/O error
	var sessionErr error
	select {
	case <-ctx.Done():
		sessionErr = ctx.Err()
	case <-p.stopCh:
		// Graceful: send DPR
		p.setState(StateClosing)
		sendDPR(p)
		select {
		case <-p.dprDoneCh:
		case <-time.After(5 * time.Second):
			p.log.Warn("DPA timeout, forcing close")
		}
	case err := <-readErrCh:
		sessionErr = err
	case err := <-writeErrCh:
		sessionErr = err
	}

	close(wdDone)
	conn.Close()
	return sessionErr
}

// nextBackoff returns the next backoff duration, capped at max.
func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}

// resolveForDial resolves addr (IP or hostname) + port to "ip:port" for dialing.
// If addr is already an IP, returns it immediately without DNS.
func resolveForDial(ctx context.Context, addr string, port int) (string, error) {
	if ip := net.ParseIP(addr); ip != nil {
		return fmt.Sprintf("%s:%d", ip.String(), port), nil
	}
	ips, err := net.DefaultResolver.LookupHost(ctx, addr)
	if err != nil {
		return "", fmt.Errorf("DNS lookup %q: %w", addr, err)
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("DNS lookup %q: no results", addr)
	}
	return fmt.Sprintf("%s:%d", ips[0], port), nil
}
