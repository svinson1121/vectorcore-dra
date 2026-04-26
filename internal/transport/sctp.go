package transport

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"syscall"

	"github.com/ishidawataru/sctp"
)

// SCTP is a plain SCTP transport using the ishidawataru/sctp library.
type SCTP struct {
	qos QoSPolicy
}

// NewSCTP creates a new plain SCTP transport.
func NewSCTP(qos ...QoSPolicy) *SCTP {
	policy := DefaultQoS()
	if len(qos) > 0 {
		policy = qos[0]
	}
	return &SCTP{qos: policy}
}

// Dial establishes an SCTP connection to addr ("host:port").
func (t *SCTP) Dial(ctx context.Context, addr string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("sctp: invalid addr %q: %w", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("sctp: invalid port %q: %w", portStr, err)
	}

	ips, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("sctp: resolving %q: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("sctp: no addresses for %q", host)
	}

	addrs := make([]net.IPAddr, 0, len(ips))
	for _, ip := range ips {
		parsed := net.ParseIP(ip)
		if parsed != nil {
			addrs = append(addrs, net.IPAddr{IP: parsed})
		}
	}

	sctpAddr := &sctp.SCTPAddr{IPAddrs: addrs, Port: port}

	// Dial with context deadline if set
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		socketCfg := &sctp.SocketConfig{
			Control: func(network, address string, c syscall.RawConn) error {
				if err := t.qos.Control(network, address, c); err != nil {
					return err
				}
				if t.qos.Preserve() {
					return EnableReceiveTOSControl(c)
				}
				return nil
			},
		}
		conn, err := socketCfg.Dial("sctp", nil, sctpAddr)
		ch <- result{conn, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		if err := t.qos.Apply(r.conn); err != nil {
			r.conn.Close()
			return nil, err
		}
		if t.qos.Preserve() {
			r.conn = WrapPreserveSCTP(r.conn)
		}
		return r.conn, nil
	}
}

// Listen creates an SCTP listener on addr ("host:port").
func (t *SCTP) Listen(addr string) (net.Listener, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("sctp: invalid listen addr %q: %w", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("sctp: invalid port %q: %w", portStr, err)
	}

	var addrs []net.IPAddr
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		// Wildcard - pass empty IPAddrs list; library binds to all interfaces (IPv4+IPv6)
		addrs = nil
	} else {
		ip := net.ParseIP(host)
		if ip == nil {
			return nil, fmt.Errorf("sctp: listen address must be an IP, got %q", host)
		}
		addrs = []net.IPAddr{{IP: ip}}
	}

	sctpAddr := &sctp.SCTPAddr{IPAddrs: addrs, Port: port}
	socketCfg := &sctp.SocketConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			if err := t.qos.Control(network, address, c); err != nil {
				return err
			}
			if t.qos.Preserve() {
				return EnableReceiveTOSControl(c)
			}
			return nil
		},
	}
	ln, err := socketCfg.Listen("sctp", sctpAddr)
	if err != nil {
		return nil, fmt.Errorf("sctp: listen on %s: %w", addr, err)
	}
	if err := t.qos.Apply(ln); err != nil {
		ln.Close()
		return nil, fmt.Errorf("sctp: setting qos on listener: %w", err)
	}
	return ln, nil
}

// Protocol returns "sctp".
func (t *SCTP) Protocol() string {
	return "sctp"
}
